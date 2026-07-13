# Telegram migration rework guard R1 — TASK-260712-2xkyot

The prior outcome is rejected and stale. Preserve all sibling onboarding and
identity-foundation edits in the shared worktree. Do not commit or push.

## T1 — exact rolling attempt limiter

`telegramLinkAttemptLimiter` currently implements a fixed window. Replace it
with a concurrency-safe 15-minute rolling window keyed by verified Telegram
user ID. Every syntactically valid attempt, including every rejected/429
equivalent attempt, advances the window. Keep per-key state bounded to the
minimum exact history and retain the 10,000-key LRU cap.

Add deterministic fake-clock and concurrency tests proving: attempts 1–10 are
allowed, 11 is rejected, later rejected attempts advance the boundary, a
fixed-window boundary burst cannot obtain 20 admissions, concurrent callers
obtain exactly 10 admissions, and LRU eviction remains bounded. Do not use
wall-clock sleeps as the proof of limiter semantics.

## T2 — uniform pre-mutation credential path

Rev15 section 9 forbids a materially different failure fast path. The current
unknown/expired/consumed code path performs one lookup, while a present code
with invalid issuer authority performs an additional lookup. Refactor the
pre-mutation gate to one fixed-shape production read that obtains code state
and issuer actor/membership/orbit authority without filtering away expired,
invalidated, or consumed rows. On a miss, use dummy fields. Always execute one
constant-time submitted-code hash comparison before combining validity.

Unknown, valid-shape malformed/guessed, expired, invalidated, consumed,
issuer-revoked, issuer-left, issuer-downgraded, disabled-orbit, and tampered-role
failures must return the same adapter error and leave all state untouched.
Add an instrumented structural test for equal read/hash operations; do not use
flaky timing thresholds. Approved same-orbit and foreign-orbit conflicts remain
visible only after a genuinely valid code passes this gate.

## T3 — Telegram transport error redaction

`net/http` errors commonly include the request URL. Telegram Bot API URLs
contain the bot token. No error reachable from `GetUpdates`, `SendMessage`,
`DeleteMessage`, `getFile`, or media download may expose that token when the
caller logs the error. Sanitize at the HTTP adapter boundary without echoing
the URL, bot token, link code, message text, or request parameters. Preserve a
useful operation name and the non-secret underlying cause. Add injected
transport-error tests that use a sentinel token and assert it and all
secret-bearing URL fragments are absent from returned errors and captured
logs, including best-effort delete failure.

## Evidence and handoff

Re-run the focused tests repeatedly, full uncached coordinator suite, exact
previous-head compatibility suite, race detector, vet, build, gofmt, secret
scan, diff review, and `task-board validate`. Replace the stale task outcome
with exact SHA-256 hashes and unabridged command results. Explicitly identify
concurrent sibling-file drift. Leave the task in `to-review`; acceptance still
requires a fresh independent security/compatibility review and root line-by-line
hash/test audit.
