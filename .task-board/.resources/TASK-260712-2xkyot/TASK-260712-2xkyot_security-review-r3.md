# Independent security/compatibility review R3 — TASK-260712-2xkyot

## Verdict

**BACK TO DEVELOPMENT.** One high-severity release blocker remains. The Telegram migration, authorization, link-consume, concurrency, compatibility, limiter, and transport-redaction implementations otherwise matched the reviewed Rev15 contract and passed the independent verification listed below.

## Release blocker

### F1 — High — Telegram rate-limit responses are not durably audited

Contract: frozen Rev15 section 12 requires every successful consume and every `429 too_many_attempts` event to be audited without plaintext secret material.

Evidence:

- `coordinator/internal/store/identity_telegram.go:123-127` reserves the rejected attempt, calls `s.LogEvent(...)`, then returns `ErrTelegramLinkRateLimited`.
- `coordinator/internal/store/store.go:302-307` identifies `events` as a debugging log; `LogEvent` writes to that legacy table and ignores both JSON and SQLite errors.
- `coordinator/internal/store/identity_schema.go:113-119` defines the additive identity audit authority as `audit_events`, but the 429 path never writes it.
- `coordinator/internal/store/identity_telegram_test.go:678-700` asserts a row in `events`; it neither asserts the identity audit repository nor injects an audit-write failure.

Concrete failure schedule:

1. A verified Telegram user submits eleven syntactically valid link-code attempts inside the 15-minute rolling window.
2. The limiter correctly rejects attempt eleven.
3. An `events` insert fails (for example, a `BEFORE INSERT` trigger raises `ABORT`, the database is full, or the write otherwise errors).
4. `LogEvent` discards the error; `ConsumeTelegramLink` still returns `ErrTelegramLinkRateLimited`, and the bot renders the rate-limit response.
5. No durable audit row exists. Even when the best-effort insert succeeds, the record is in the legacy debugging table rather than `audit_events`.

Impact: the authorization decision remains fail-closed and no credential is disclosed, but a mandatory security event can disappear silently. This violates the frozen contract and removes accountable evidence for brute-force throttling. Tests are green because they encode the incorrect best-effort sink.

Minimal remedy: provide one shared durable audit path that can represent pre-identity/global rate-limit events without plaintext credentials, return/handle persistence errors instead of discarding them, route Telegram consume 429s through it, and add a fault-injected test proving a rate-limit response cannot be emitted without the required audit record. Coordinate the schema/repository shape with onboarding, whose pre-identity 429 paths have the same representational need. Preserve the rolling limiter reservation order and do not add a credential lookup before reservation.

## Reviewed inventory and conclusions

I read the complete Rev15 contract, root amendments, identity-foundation acceptance/review evidence, all Telegram implementation/R1/R2/R3 guards, the R2 producer outcome, both supplied PlantUML diagrams, the requested specification sections, and the current production/test files for Telegram consume, identity/schema/reconciliation, onboarding issuance, legacy orbit mutations, bot parser/transport, coordinator loop, and exact-previous-head harness.

Architecture validation used the diagrams as two independent invariants: the class view kept app-installation credential ownership separate from Telegram actors, while the sequence view required trusted in-process bot consume and additive legacy coexistence. The implementation satisfied those invariants: there is no public HTTP consume route; the only production caller is the long-polling loop; private-chat/update principals are enforced; commands resolve through shared `ActorContext`; linking creates/reuses only a Telegram actor and does not transfer installation credentials; legacy roles, slots, pair/share/leave/revoke/transfer paths remain coherent.

No additional blocker was found in the following challenged areas:

- exact feature-off behavior, migrated primary/companion/satellite roles, slot ownership, old node-token scope, and mixed legacy/self-service pairing;
- issuance re-auth, desired-role policy, fixed-shape pre-mutation read, one constant-time comparison, dummy target, uniform invalid errors, expiry/invalidation/replay, and visible conflicts only after a valid credential gate;
- exact rolling 10-per-15-minute limiter semantics, rejected-attempt advancement, atomicity, bounded ten-timestamp/key state, and 10,000-key LRU cap;
- `_txlock=immediate`, all consume reads/writes through one transaction, conditional reservation, conflict-safe UPSERTs, legacy dual-write, rollback, partial-unique defense, and absence of `INSERT OR REPLACE` from production identity mutation;
- R2 concurrency proof: writer two signals at `telegram_link_transaction_attempting` immediately before the real `db.Begin()`, cannot reach preflight while writer one is held, and preserves the loser code in the two-code/same-user case;
- Telegram HTTP redirect fail-closed behavior and redaction of URL, token, form/message/file/path material from rendered errors, unwrap graphs, and logs; best-effort deletion runs only after commit and does not echo the code.

## Hash boundary

The two mandatory R2 hashes match the producer outcome exactly:

```text
583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go
a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go
```

Additional reviewed boundary:

```text
3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go
a94852e8dbed50e4faf5ae45569247022c09d6c2c04bab8696627d11f5033d8c  coordinator/internal/bot/bot.go
96718ef5a213327de1240985a4205a1c949f67dcf22cd32be7b230f8ea5260fb  coordinator/internal/bot/bot_test.go
7a4786c970b97e6c4992bbec3b46be1445a6a601b4ab586566a3eaf4d866760a  coordinator/cmd/duet-coordinator/loop.go
dcd4cc3c1188569439335c1742c657cb4235aec223f1d2ed5f4cb4fcde0de5dd  coordinator/internal/store/identity.go
892238d4d8d6aa3adbeb7c9a1009df693d84fb9803c5fa21b718521ca33472bb  coordinator/internal/store/identity_schema.go
66948069ac47d7a8f2f21718149f129a1ff89bba8b47b20fe863a550e5c1ea43  coordinator/internal/store/onboarding.go
63bd1e1717fd4c964aff470ab3af05932aa66e0b8543372bc9c1a2aa25cc8450  coordinator/internal/store/orbits.go
```

## Independent command results

```text
$ cd coordinator && go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
ok  relux.works/duet/coordinator/internal/store  12.848s

$ cd coordinator && go test -count=20 ./internal/store -run '^(TestTelegram|TestConsumeTelegram|TestLinkedTelegram)'
ok  relux.works/duet/coordinator/internal/store  17.787s

$ cd coordinator && go test -count=20 ./internal/bot
ok  relux.works/duet/coordinator/internal/bot  1.082s

$ cd coordinator && go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegram|TestMigratedTelegram|TestAppOwnedOrbitTelegram)'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  3.537s

$ cd coordinator && go test -count=1 ./...
ok  relux.works/duet/coordinator/cmd/duet-coordinator  2.790s
ok  relux.works/duet/coordinator/internal/bot  2.025s
ok  relux.works/duet/coordinator/internal/config  0.684s
ok  relux.works/duet/coordinator/internal/hub  1.051s
ok  relux.works/duet/coordinator/internal/links  2.578s
ok  relux.works/duet/coordinator/internal/media  2.940s
ok  relux.works/duet/coordinator/internal/protocol  2.635s
ok  relux.works/duet/coordinator/internal/resolver  2.737s
ok  relux.works/duet/coordinator/internal/session  2.177s
?   relux.works/duet/coordinator/internal/spotify  [no test files]
ok  relux.works/duet/coordinator/internal/store  6.943s
?   relux.works/duet/coordinator/internal/ulid  [no test files]

$ cd coordinator && go test -tags previoushead -count=1 ./internal/store
ok  relux.works/duet/coordinator/internal/store  14.573s
```

The previous-head test resolves and archives exact local revision `e8bd240664a40b9cc78b974f3c34ad30712e2aa5`, injects only its authority-surface driver, executes that source, then reopens/reconciles with the current store.

```text
$ cd coordinator && go test -race -count=1 ./...
ok  relux.works/duet/coordinator/cmd/duet-coordinator  11.919s
ok  relux.works/duet/coordinator/internal/bot  1.436s
ok  relux.works/duet/coordinator/internal/config  2.201s
ok  relux.works/duet/coordinator/internal/hub  1.820s
ok  relux.works/duet/coordinator/internal/links  3.096s
ok  relux.works/duet/coordinator/internal/media  3.398s
ok  relux.works/duet/coordinator/internal/protocol  2.480s
ok  relux.works/duet/coordinator/internal/resolver  2.618s
ok  relux.works/duet/coordinator/internal/session  2.632s
?   relux.works/duet/coordinator/internal/spotify  [no test files]
ok  relux.works/duet/coordinator/internal/store  27.592s
?   relux.works/duet/coordinator/internal/ulid  [no test files]

$ cd coordinator && go vet ./...
PASS (no output)

$ cd coordinator && go vet -tags previoushead ./internal/store
PASS (no output)

$ cd coordinator && go build ./...
PASS (no output)

$ cd coordinator && rg --files . -g '*.go' -0 | xargs -0 gofmt -l
PASS (no output)

$ git diff --check
PASS (no output)

$ rg -n 'INSERT OR REPLACE' coordinator/internal/store/identity*.go coordinator/internal/store/onboarding.go
Only two test-fixture legacy-slot mutations were found; no production identity mutation uses it.

$ rg -n 'HandleFunc\\(|ConsumeTelegramLink\\(' coordinator/cmd coordinator/internal --glob '*.go'
Only `/v1/telegram-links` issuance is registered; the sole production consume call is `cmd/duet-coordinator/loop.go:1391`.

$ task-board validate
Board is valid. No issues found.
```

## Worktree and external boundaries

The worktree was already dirty and was preserved. Relevant tracked changes include the bot, loop, orbit/store integration, config, CI, docs, and logbook; identity/onboarding/schema and their tests are untracked sibling work. Unrelated board/planning/Windows work is also present. The stable Telegram/shared hashes above match the producer boundary; the known later sibling drift is outside those hashes. This review does not accept onboarding or Windows work.

No live Telegram service, real bot token, or external network behavior was exercised. Transport behavior was verified through the production HTTP adapter with injected clients/servers. No production code, tests, checklist, commit, or push was changed by this review.
