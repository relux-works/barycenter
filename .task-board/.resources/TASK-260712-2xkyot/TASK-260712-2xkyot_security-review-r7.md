PASS

# TASK-260712-2xkyot independent Telegram security and compatibility review R7

Date: 2026-07-13
Role: independent reviewer
Mode: read-only review of the combined working tree
Producer boundary: Telegram R6

## Reviewed inventory

Authoritative inputs were read in full where applicable:

- frozen Rev15 identity/recovery contract and root amendments for TASK-260712-3v1k7q;
- the identity and onboarding PlantUML attachments;
- docs/spec-self-contained-audio.md sections 3/3.13, 6, 11, 12, 18, and 19;
- the accepted identity-foundation implementation and review evidence for TASK-260712-1bpog0;
- all Telegram implementation/rework guards through R6;
- the prior R3 and R5 independent findings;
- the R6 producer outcome;
- the R7 review mandate.

Current production and test inventory reviewed line by line or as an adjacent shared dependency:

- coordinator/internal/bot/bot.go and bot_test.go;
- coordinator/internal/bot/commands.go and commands_test.go;
- coordinator/cmd/duet-coordinator/loop.go, main.go, onboarding.go, and telegram_identity_test.go;
- coordinator/internal/store/identity_telegram.go, identity_telegram_test.go, and identity_telegram_previous_head_test.go;
- coordinator/internal/store/identity.go, identity_schema.go, onboarding.go, security_audit.go, store.go, and the relevant legacy mutation/reconciliation paths in orbits.go;
- the pinned previous-head driver and compatibility fixtures.

The identity class diagram was used as the ownership invariant: Telegram linkage adds a Telegram actor and membership but does not mutate AppInstallationCredential ownership. The onboarding sequence diagram was used to verify the trusted in-process Bot consume boundary and the additive legacy/self-service flow.

## Frozen boundary hashes

All R7 hashes matched before the review and again after the final test run:

- 96d295381a10197506eee4bf0d99adb7f0a9ecbf04bc3abb596e929f33fa5b04  coordinator/internal/bot/bot.go
- 96638935ed384bd6ff99a776bcd6b505eb39a96b0aafaeabb3625355411db04b  coordinator/internal/bot/bot_test.go
- 175c65f22c92649d27140964f911b1b7deb9621a2e1361301b66ccba8481b1ac  coordinator/cmd/duet-coordinator/telegram_identity_test.go
- 1d99a568881d5bc22b53166a9d76cd04d6bae10ef59a53c27d39d5b1dab72451  coordinator/internal/store/identity_telegram.go
- 17b046bc1202f632f8082d7121ae40e835106de5ddd4ecf2fe14794887f07c4d  coordinator/internal/store/identity_telegram_test.go
- efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
- 90940d1252d9a44b6174bb7482b8a71aed522c450022321c02003e3c3f6137c1  coordinator/cmd/duet-coordinator/loop.go
- 840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134  coordinator/internal/store/identity.go
- 6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f  coordinator/internal/store/identity_schema.go
- 77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9  coordinator/internal/store/onboarding.go
- 194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa  coordinator/internal/store/security_audit.go
- 9a04b44784201d11ad688ae624f3202343946d16ec37746bf59a0c2205c5cd16  TASK-260712-2xkyot_rework-r6-results.md

No frozen-file drift was observed.

## Security and compatibility conclusions

1. Durable rate-limit classification and ordering pass.

coordinator/internal/store/identity_telegram.go:110-140 enforces feature state, verified positive Telegram principal/private-chat classification, syntax normalization, atomic rolling reservation, durable typed audit on rejection, and only then the transaction attempt. ErrTelegramLinkRateLimited is returned only after RecordRateLimitAudit succeeds. A durable insert error returns a constant structural wrapper, retains the already-reserved attempt, and cannot map to the rate-limit or credential/conflict sentinels.

coordinator/internal/store/security_audit.go:10-115 applies the class-domain-separated digest, stores security.rate_limited with the exact telegram-link-consume/telegram-user class, and uses NULL actor/orbit scope for this pre-identity limiter. No production Telegram consume call writes the legacy events sink.

2. The rolling limiter passes.

coordinator/internal/store/identity_telegram.go:40-101 serializes reservation under one mutex, keeps exactly the newest ten timestamps per key, advances the boundary for rejected attempts, applies the exact fifteen-minute cutoff, and bounds keys with a 10,000-entry LRU. Syntax-invalid attempts return before reservation. Deterministic fake-clock, boundary-burst, concurrent exact-ten, rejected-attempt, and LRU tests cover these properties.

3. Credential gate and transaction linearization pass.

coordinator/internal/store/identity_telegram.go:135-209 places the production-neutral transaction-attempt checkpoint immediately before the real db.Begin. The Store DSN uses _txlock=immediate. The code-state/issuer-actor/membership/orbit lookup is one fixed-shape transaction read, a miss receives a dummy digest, and exactly one constant-time submitted-code comparison occurs before validity is combined. Unknown, expired, invalidated, consumed, revoked/left/downgraded issuer, disabled orbit, and tampered role remain the same credential error without mutation.

coordinator/internal/store/identity_telegram.go:215-305 performs actor resolution, additive and legacy conflict checks, conditional single-use reservation, membership and legacy UPSERT, and audit through the same transaction. No INSERT OR REPLACE identity mutation exists. Rollback tests cover legacy write and audit failure.

The two independent-connection concurrency tests signal writer two from the exact checkpoint before db.Begin, hold writer one after the credential lookup, assert writer two cannot complete or reach credential preflight while writer one owns the immediate transaction, and then verify one winner. The two-code/same-user case also verifies the losing code remains unconsumed. Fifty repetitions passed.

4. Trusted boundary, role authorization, and ownership pass.

The only production caller of Store.ConsumeTelegramLink is coordinator/cmd/duet-coordinator/loop.go:1391. It receives principal, chat type, message, and display metadata only from Bot.Run processing an authenticated Telegram Update. Non-private consumes are rejected before Store mutation. There is no public HTTP consume route; /v1/telegram-links is the control-credential-protected issuance route only.

coordinator/cmd/duet-coordinator/loop.go:1356-1415 routes feature-on bot authorization through ResolveTelegramActorContext and preserves the frozen unknown/left/revoked/disabled lifecycle classifications. Feature-off authorization remains the exact legacy MemberOf path. Issuer roles and desired roles are rechecked in the writer transaction. Same-orbit and foreign-orbit conflicts become visible only after a valid credential gate.

Telegram consume touches no installation credential or slot ownership row. App ownership, node/control capability separation, migrated roles, slot ownership, legacy playback tokens, pair/share/leave/revoke/primary-transfer behavior, and mixed legacy plus self-service flows are covered by Store and real-loop tests.

5. R6 transport/privacy correction passes.

coordinator/internal/bot/bot.go:145-169 and 199-254 make SendTo overflow, reply failure, source-delete failure, and source-delete overflow log constant operation fields only. Source deletion remains asynchronous and non-authoritative.

coordinator/internal/bot/bot.go:284-367 reduces transport/filesystem causes to safe sentinels. telegramOperationError unwraps only a safe cause. safeTelegramLogError finds an already-sanitized operation error through nested/joined wrappers and discards unsafe outer wrappers; arbitrary alternate API errors are classified rather than retained. coordinator/internal/bot/bot.go:373-494 clones injected clients, overrides redirects fail-closed, and sanitizes request, response, URL, token, body, Telegram description, file identifier, and destination-path failures.

The direct bot tests use sentinel private-chat and distinct message identifiers, complete structured slog capture, rendered URL/form/body/path canaries, and full reachable error-graph traversal. The real Bot integration tests at coordinator/cmd/duet-coordinator/telegram_identity_test.go:654-846 exercise durable-audit success and injected persistence failure, committed consume followed by DeleteMessage/SendMessage failure, and deterministic exact-capacity queue saturation through bot.New, Bot.Run, loop, and Store. Identity commit survives asynchronous failure, and captured logs/replies omit all canaries.

6. Migration, reconciliation, and previous-head compatibility pass.

Legacy Telegram members reconcile into additive actors and memberships with their existing orbit and role. Legacy member rows remain the rollback authority for old-binary intervals, while app-first installation ownership is not assigned to Telegram. Role transfer, leave/dissolve, revoke, paired-slot behavior, migrated primary/companion/satellite roles, and mixed pair flows are covered.

The previous-head test runs the pinned local prior Store implementation against a database produced by the current implementation, then reopens it with current code and verifies role/leave/transfer/code state and database health. It is not inferred from current-only tests.

## Independent executable evidence

Commands were run from coordinator/ unless stated otherwise.

1. go test -count=20 ./internal/bot -run '^(TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestHTTPAPIRedirectsFailClosedWithoutReachingTarget|TestHTTPAPIFilesystemErrorGraphDoesNotExposeDestinationPath|TestHTTPAPIRejectedResponseDoesNotEchoTelegramDescription|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
Exit 0: internal/bot passed in 0.446s.

2. go test -race -count=10 ./internal/bot -run '^(TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
Exit 0: internal/bot passed in 1.444s.

3. go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$'
Exit 0: cmd/duet-coordinator passed in 1.391s.

4. go test -race -count=10 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$'
Exit 0: cmd/duet-coordinator passed in 5.889s.

5. go test -count=20 ./internal/store -run '^(TestTelegramMigration|TestConsumeTelegramLink|TestLinkedTelegram|TestTelegramLinkAttemptLimiter|TestTelegramLinkIssue|TestTelegramResolver)'
Exit 0: internal/store passed in 18.933s.

6. go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
Exit 0: internal/store passed in 13.138s.

7. go test -race -count=10 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiterConcurrentReservationsAndLRUBound)$'
Exit 0: internal/store passed in 6.731s.

8. go test -count=1 ./...
Exit 0: every coordinator package passed; internal/store passed in 6.031s.

9. go test -tags previoushead -count=1 ./internal/store
Exit 0: pinned previous-head compatibility passed in 12.741s.

10. go test -race -count=1 ./...
Exit 0: every coordinator package passed under the race detector; internal/store passed in 31.987s.

11. go vet ./...
Exit 0 with no output.

12. go vet -tags previoushead ./internal/store
Exit 0 with no output.

13. go build ./...
Exit 0 with no output.

14. rg --files -g '*.go' -0 | xargs -0 gofmt -l
Exit 0 from coordinator with no output: coordinator Go formatting is clean.

15. git diff --check
Exit 0 with no output.

16. task-board validate
Exit 0: Board is valid. No issues found.

17. Production call-graph and sink scans:
- ConsumeTelegramLink production scan found only its Store definition and loop.go:1391 caller.
- HTTP route scan found only the authenticated issuance route /v1/telegram-links; no consume route.
- reviewed identity production files contain no INSERT OR REPLACE.
- the legacy telegram_link.rate_limited string occurs only in negative tests; Telegram consume uses RecordRateLimitAudit.
- Telegram bot log-field scan found constant sendMessage/deleteMessage operations and no private Telegram identifier fields.
Exit status was 0 for positive-match scans and 1 for the expected no-match INSERT OR REPLACE scan.

Command corrections were recorded rather than hidden. An initial zsh command-substitution form of the gofmt scan exited 2 with a file-name-too-long invocation error; the NUL-delimited xargs command above is the corrected successful proof. An initial set of root-relative rg scans exited 2 because it was run from the repository root with coordinator-relative paths; the same scans were rerun from coordinator and succeeded. A repository-root formatting scan reported two out-of-scope Windows files; it made no edits and the required coordinator scope is clean.

## Findings

No release blocker, high, medium, low, or informational implementation defect was found in the reviewed Telegram scope.

Observations and evidence boundaries:

- The worktree is intentionally dirty with concurrent sibling onboarding, identity, application, documentation, and Windows work. It was preserved. The exact frozen Telegram/shared hashes did not drift.
- This review does not accept or assess the sibling onboarding client or Windows deliverables beyond verifying that frozen shared dependencies remained unchanged.
- Tests used isolated temporary SQLite databases and injected Telegram HTTP/API implementations. No live Telegram call, production database, external CI, commit, or push was performed.
- The whole-file SHA-256 of this report is recorded immediately after attachment in the reviewer handoff/board note; a file cannot contain its own final digest without changing that digest.
