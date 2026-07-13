# Telegram migration rework guard R2 — TASK-260712-2xkyot

The R1 outcome remains rejected and stale. Preserve every R1 behavior and all
sibling onboarding/identity edits in the shared worktree. Do not commit or push.
This run is intentionally narrow: repair the missing deterministic proof below,
then refresh evidence. Do not reinterpret or relax the Rev15 contract.

## T10 — prove the second writer actually attempts `BEGIN IMMEDIATE`

The current concurrency tests launch writer 2 and immediately use a 100 ms
negative assertion. They never establish that writer 2 reached the transaction
attempt. A scheduler delay can therefore let a broken deferred/non-serializing
implementation pass.

Add a production-neutral test checkpoint immediately before `s.db.Begin()` in
`consumeTelegramLinkAt` (for example
`telegram_link_transaction_attempting`). In both concurrency tests, configure
the second `Store` instance to signal only from this exact checkpoint. Wait for
that signal before asserting that writer 2 cannot return while writer 1 owns the
transaction. Preserve the existing first-writer post-lookup hold, result
channels, one-winner/one-loser assertions, and loser-code-unconsumed assertion.

The proof must fail if transaction acquisition is moved after the credential
read or changed to a deferred/non-serializing transaction. The checkpoint must
not expose credentials, URLs, or request data and must not alter production
behavior when no test hook is installed.

Review the adjacent concurrency tests for the same pre-attempt ambiguity and
fix any occurrence within this task's files. Coordinate with the live onboarding
run: it may introduce an equivalent shared checkpoint convention. Re-read the
settled shared files immediately before editing and preserve non-task changes.

## Required verification and handoff

Run the two concurrency tests repeatedly, all focused Telegram identity/link
tests, the full uncached coordinator suite, race detector, vet, build, gofmt,
diff review, credential/URL leakage scans, and `task-board validate`. Replace
the stale task-scoped outcome with exact hashes and complete command results.
Leave the task in `to-review`; root acceptance still requires line-by-line
review, independent reruns, and a separate security/compatibility reviewer.
