# TASK-260712-2xkyot — Telegram migration R2 producer evidence

Date: 2026-07-13 (Asia/Tbilisi)
Role: developer
Handoff target: `to-review`

## Outcome

R2 adds a production-neutral `telegram_link_transaction_attempting` checkpoint
immediately before `consumeTelegramLinkAt` calls `s.db.Begin()`. The shared
`Store.checkpoint` hook is nil in production, receives only a constant boundary
name, and does not expose a credential, Telegram update, URL, or request field.

Both independent-connection consume races now configure writer two to signal
only from that exact checkpoint. The tests wait for the signal before the
existing 100 ms negative assertion. They also make the existing
`telegram_link_preflight_read` checkpoint return a test-only sentinel if writer
one has not been released. Therefore:

- moving acquisition after the credential read makes writer two return before
  the transaction-attempt signal and fails the test;
- changing acquisition to deferred/non-serializing lets writer two pass
  `Begin`, reach preflight, and return the sentinel during the negative
  assertion, failing the test; and
- the current immediate transaction blocks writer two inside `Begin` until
  writer one releases, after which the original winner/loser semantics run.

The first writer remains held at `telegram_link_after_lookup`. Same-code/two-user
still has one success and one generic credential failure. Two-code/same-user
still has one success, one same-orbit conflict, and the loser code remains
unconsumed. R1 rolling-limit, fixed-shape credential preflight, ActorContext,
transport redaction, legacy dual-write, feature-off, and exact-old behavior are
unchanged.

## R2 files and exact SHA-256

```text
583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go
a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go
497f159470e500d070276fbe4f54a272247a1945530faa48cf467f7f78ee62ac  LOGBOOK.md
```

Pre-R2 R1 hashes for the two changed Go files were:

```text
1f120119f7cc5fbce7ad53aeeb8c51b6ec017a7d6488848b90ac4d87d99c6763  coordinator/internal/store/identity_telegram.go
6d4df3fe970c5836fd4ca1bcdf7b080f878bd25e353c4b1b0b6efce57a7d5788  coordinator/internal/store/identity_telegram_test.go
```

Stable R1/accepted-foundation dependencies at the same verification boundary:

```text
18b19ab33911052ef653fa4bac1a80671c4ea9fba1db37f4f8d08637f3f85799  coordinator/internal/store/store.go
dcd4cc3c1188569439335c1742c657cb4235aec223f1d2ed5f4cb4fcde0de5dd  coordinator/internal/store/identity.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
79fe0c815065e3fd3ce8c374efeceac8693b3a5d72d7cb77160bc2c5dc8242b6  coordinator/internal/bot/commands.go
a94852e8dbed50e4faf5ae45569247022c09d6c2c04bab8696627d11f5033d8c  coordinator/internal/bot/bot.go
96718ef5a213327de1240985a4205a1c949f67dcf22cd32be7b230f8ea5260fb  coordinator/internal/bot/bot_test.go
7a4786c970b97e6c4992bbec3b46be1445a6a601b4ab586566a3eaf4d866760a  coordinator/cmd/duet-coordinator/loop.go
3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go
```

## Requirement and test mapping

| Requirement | Production/test proof |
|---|---|
| T10 exact attempt checkpoint | `consumeTelegramLinkAt` calls the constant checkpoint immediately before `s.db.Begin()` |
| Writer two reached the attempt | both conflict tests wait on a channel closed only by `telegram_link_transaction_attempting` on `s2` |
| Immediate acquisition, not later write blocking | preflight-read sentinel returns before writer-one release if `Begin` is deferred/non-serializing |
| Acquisition cannot move after credential read | a preflight read before the attempt returns the sentinel; writer two completes before the expected attempt signal and the test fails |
| Same-code single winner | `TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner` |
| Two-code/same-user loser code preserved | `TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode` |
| Rev15/R1 behavior retained | focused Telegram store/bot/coordinator repetitions, full uncached suite, exact previous-head suite, and race suite |
| Secret/URL boundary | no plaintext-code, credential-bearing URL, `INSERT OR REPLACE`, or public HTTP consume matches in the recorded scans |

## Verification commands and results

Working directory for Go commands: `coordinator/`.

### Initial focused execution

```text
$ go test -count=1 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$' -v
=== RUN   TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner
--- PASS: TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner (0.12s)
=== RUN   TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode
--- PASS: TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode (0.12s)
PASS
ok  relux.works/duet/coordinator/internal/store  0.631s
```

### R2 concurrency repetitions

```text
$ go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
ok  relux.works/duet/coordinator/internal/store  12.995s
```

### Focused Telegram identity/link, transport, and coordinator repetitions

```text
$ go test -count=20 ./internal/store -run '^(TestTelegramMigration|TestConsumeTelegramLink|TestLinkedTelegram|TestTelegramLinkAttemptLimiter|TestTelegramLinkIssue|TestTelegramResolver)'
ok  relux.works/duet/coordinator/internal/store  18.528s

$ go test -count=20 ./internal/bot -run '^(TestTelegramLinkCodeCarriesVerifiedUpdateMetadata|TestParseTelegramLinkCodeNormalizationShape|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestHTTPAPIRedirectsFailClosedWithoutReachingTarget|TestHTTPAPIFilesystemErrorGraphDoesNotExposeDestinationPath|TestHTTPAPIRejectedResponseDoesNotEchoTelegramDescription|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
ok  relux.works/duet/coordinator/internal/bot  0.467s

$ go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegramLinkHTTPContractHasNoSecretURL|TestMigratedTelegramActorContextKeepsLegacyAndSelfServicePairCompatibility|TestTelegramActorLifecycleControlsStrangerOnboarding|TestAppOwnedOrbitTelegramLinkUsesTrustedAdapterAndPreservesPairOwnership|TestTelegramLinkFeatureOffKeepsCodeShapedChatterSilent)$'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  3.596s
```

### Full uncached combined coordinator suite

```text
$ go test -count=1 ./...
ok  relux.works/duet/coordinator/cmd/duet-coordinator  1.307s
ok  relux.works/duet/coordinator/internal/bot  1.327s
ok  relux.works/duet/coordinator/internal/config  0.675s
ok  relux.works/duet/coordinator/internal/hub  1.631s
ok  relux.works/duet/coordinator/internal/links  0.965s
ok  relux.works/duet/coordinator/internal/media  1.885s
ok  relux.works/duet/coordinator/internal/protocol  1.537s
ok  relux.works/duet/coordinator/internal/resolver  1.577s
ok  relux.works/duet/coordinator/internal/session  1.526s
?   relux.works/duet/coordinator/internal/spotify  [no test files]
ok  relux.works/duet/coordinator/internal/store  5.477s
?   relux.works/duet/coordinator/internal/ulid  [no test files]
```

### Exact pinned previous-head compatibility

```text
$ go test -tags previoushead -count=1 ./internal/store
ok  relux.works/duet/coordinator/internal/store  10.736s
```

This ran the local pinned previous-source integration. It is not an external CI
claim.

### Race detector

```text
$ go test -race -count=1 ./internal/store ./internal/bot ./cmd/duet-coordinator
ok  relux.works/duet/coordinator/internal/store  24.448s
ok  relux.works/duet/coordinator/internal/bot  1.733s
ok  relux.works/duet/coordinator/cmd/duet-coordinator  10.095s
```

### Static analysis and build

```text
$ go vet ./...
PASS (exit 0; no output)

$ go vet -tags previoushead ./internal/store
PASS (exit 0; no output)

$ go build ./...
PASS (exit 0; no output)
```

### Formatting and diff validation

```text
$ rg --files . -g '*.go' -0 | xargs -0 gofmt -l
PASS for the coordinator module (exit 0; no output)

$ git diff --check
PASS (exit 0; no output)
```

Repository-root `gofmt -l` was also run. It reported the unrelated sibling
files `pulsar-win/player.go` and `pulsar-win/ui_windows.go`. They are outside
this task and were not rewritten. The full coordinator module, including both
R2 files, is formatting-clean.

The two task Go files are pre-existing untracked shared-worktree files, so
`git diff` cannot display a baseline hunk for them. R2 review used the complete
file read, the exact pre-R2 hashes above, the exact post-R2 hashes above,
`gofmt`, focused tests, and the full validation matrix. No unrelated hunk was
added to either file.

### Policy, secret, URL, and adapter-boundary scans

```text
$ rg -n 'INSERT[[:space:]]+OR[[:space:]]+REPLACE' coordinator/internal/store/identity_telegram.go coordinator/internal/store/identity_telegram_test.go coordinator/cmd/duet-coordinator/telegram_identity_test.go
PASS (no matches; ripgrep exit 1)

$ rg -n '\b[ABCDEFGHJKMNPQRSTVWXYZ2-9]{27}\b' coordinator/internal/store/identity_telegram.go coordinator/internal/store/identity_telegram_test.go coordinator/internal/store/identity_telegram_previous_head_test.go coordinator/internal/bot/commands.go coordinator/internal/bot/bot.go coordinator/internal/bot/bot_test.go coordinator/cmd/duet-coordinator/loop.go coordinator/cmd/duet-coordinator/telegram_identity_test.go
PASS (no plaintext link-code literals; ripgrep exit 1)

$ rg -n 'https?://[^[:space:]"`]*(bot|token)[A-Za-z0-9:_-]{16,}|api\.telegram\.org/(bot|file/bot)[A-Za-z0-9:_-]{16,}' coordinator/internal/store/identity_telegram.go coordinator/internal/store/identity_telegram_test.go coordinator/internal/bot/commands.go coordinator/internal/bot/bot.go coordinator/internal/bot/bot_test.go coordinator/cmd/duet-coordinator/loop.go coordinator/cmd/duet-coordinator/telegram_identity_test.go LOGBOOK.md .task-board/.resources/TASK-260712-2xkyot/TASK-260712-2xkyot_results.md
PASS (no credential-bearing URL literals; ripgrep exit 1)

$ rg -n 'ConsumeTelegramLink\(' coordinator --glob '*.go'
PASS: matches are limited to the Store method, the trusted in-process loop call,
and tests. No HTTP handler matched.
```

### Board validation before evidence attachment

```text
$ task-board validate
Board is valid. No issues found.
```

### Board validation after evidence attachment

```text
$ task-board validate
Board is valid. No issues found.
```

## Corrected/anomalous checks

No product or test failure occurred after the R2 edit. The only non-clean
cross-repository check was repository-root `gofmt -l`, which identified two
unrelated sibling Windows files. The relevant coordinator-wide formatting
command was run separately and returned no paths. This boundary is retained
here rather than changing sibling-owned files.

## Dirty-worktree and sibling coordination

- The repository was already broadly dirty. No reset, checkout, commit, or push
  was performed.
- R2 production/test changes are limited to the two hash-anchored Telegram
  store files. The logbook received one R2 evidence bullet.
- Concurrent onboarding run `RUN-260713-9b24a6` completed successfully before
  evidence freeze. It owned `identity.go`, both `onboarding.go` files, their
  tests, and its task outcome. R2 did not edit those files.
- The shared logbook changed after the initial verification freeze when sibling
  task entries were consolidated. The exact hash above was refreshed immediately
  before outcome attachment; the Telegram R2 evidence bullet remained unchanged.
- Stable sibling dependency hashes at the combined-suite boundary:

```text
dcd4cc3c1188569439335c1742c657cb4235aec223f1d2ed5f4cb4fcde0de5dd  coordinator/internal/store/identity.go
66948069ac47d7a8f2f21718149f129a1ff89bba8b47b20fe863a550e5c1ea43  coordinator/internal/store/onboarding.go
6aa01fd1ee8f34526ebfba9db4807e468c46850b0e13bcb38ba6510a2a3064c3  coordinator/cmd/duet-coordinator/onboarding.go
```

- `store.go`, bot adapter files, loop authorization, and previous-head Telegram
  coverage retained the R1 hashes listed above.

## External evidence boundaries

- No live Telegram Bot API, production database, production identity,
  deployment, or external GitHub Actions run was used or claimed.
- The pinned previous coordinator source was exercised locally through its real
  Store API; a separately archived production binary was not executed.
- No credential, link code, bot token, Telegram message text, request URL, or
  local secret-bearing path is included in this artifact.
- Fresh independent security/compatibility review and a root line-by-line,
  hash, and test audit remain required before downstream reliance.

The developer handoff is ready for review; this outcome does not claim reviewer
acceptance.
