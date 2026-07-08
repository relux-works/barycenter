# Architecture hardening plan

Audience: the founder, to approve and schedule. Date: 2026-07-08.

These are the larger Tier-2 items from the system-design review
(`.temp/reflection/architecture.md`, `SYNTHESIS.md`) that should **not** be
rushed in before the first real session but **must** land before onboarding
strangers / scaling past a handful of orbits. Each item below is a plan, not an
implementation: problem, proposed approach, effort, and what it unblocks.

Effort key: **S** = hours, **M** = a day or two, **L** = a week+.

Already handled elsewhere (context, not part of this plan): configurable log
level (done), prod SQLite backups via Litestream (done — `docs/backup-restore.md`),
async Telegram/resolve off the loop goroutine (Tier-1 #9), cross-tenant media
scope (Tier-0 #2).

## Priority summary

| # | Item | Effort | Blocks / unblocks |
|---|------|--------|-------------------|
| 1 | Migration framework | M | Any data-transforming schema change (the `ctid` provider migration); safe deploys |
| 2 | Metrics + alerting | M | Knowing we're down/desynced before a user tells us; growth confidence |
| 3 | Timing NFR (≤150 ms) e2e test | M | A regression net for the headline product guarantee |
| 4 | Scaling to ~100–1000 orbits | L (staged) | Onboarding strangers; the single-writer ceiling |

Recommended order: **1 → 2 → 3 → 4**. Migrations first because it is a
prerequisite for the provider (`ctid`) work and de-risks every later change; the
scaling track is last and staged, since the current scope ("dozens of orbits")
does not need it yet.

---

## 1. Migration framework (review 1.5) — effort M

**Problem.** Schema today is `CREATE TABLE IF NOT EXISTS` plus one best-effort,
error-swallowed `ALTER TABLE slots ADD COLUMN provider`
(`coordinator/internal/store/store.go`, `orbits.go`). There is no version table,
no data migrations, no down path, no way to verify a migration ran. The
invariant "prod orbits survive every migration" (`docs/goal-v2.md`) holds today
only by additive-only discipline. The already-planned
`Playlist.Tracks []uri → []ctid` change (`docs/spec-providers.md`) needs to
*transform* data — there is nowhere for it to live and no way to roll it back. A
half-applied change on prod is unrecoverable without the new backup (now in
place, but recovery is a blunt instrument).

**Proposed approach.**
- Add a `schema_version` table (single row, integer).
- An ordered list of idempotent migration funcs (`[]func(*sql.Tx) error`), each
  bumping the version, run inside one transaction at boot **before** the app
  serves traffic. Log each applied step.
- Fold the existing `CREATE TABLE IF NOT EXISTS` bodies in as migration 1 so new
  and existing DBs converge on the same path.
- No "down" migrations (SQLite makes them painful and we don't need reversibility
  in prod); recovery for a bad migration is: restore from Litestream to a point
  before it (`docs/backup-restore.md`), fix forward.
- Take an automatic `sqlite3 .backup` snapshot immediately before running any
  pending migration (the manual snapshot command already documented in the
  runbook, invoked from boot when `schema_version` is about to change).

**Unblocks.** The `ctid` provider migration; any future data reshaping; safe,
auditable deploys. Turns "hope the ALTER worked" into a logged, verifiable step.

**Sequencing note.** Do this **before** enabling `DUET_PROVIDERS`, because the
provider rollout is the first migration that transforms rather than adds.

---

## 2. Metrics + alerting (review 7.2) — effort M

**Problem.** The only production signal is `uptime.yml` curling `/healthz` every
30 minutes; its sole failure output is a failed GitHub Action email. The system
already computes desync, ready-timeouts, offline transitions and writes an
`events` table — none of it is exposed. We would not know about a system-wide
loop stall, a spike in start desync, or a climbing skip rate until a user
complains. 30-minute polling also means up to a half-hour blind window on a hard
down.

**Proposed approach.**
- Add a `/metrics` endpoint (Prometheus text format; no new heavy deps —
  `expvar` or a tiny hand-rolled exposition is enough at this size). Export:
  orbits total / active, nodes connected, per-start desync histogram (data
  already at the `EffLogDesync` sites), ready-timeout and skip counts, resolve
  latency (once providers ship), loop iteration lag, and DB write latency.
- A real alert channel that does not depend on someone reading GitHub email: the
  **bot itself DMs the founder** on `/healthz` failure and on threshold breaches
  (desync > tolerance sustained, loop lag high, litestream replication stale).
  This reuses infrastructure we already run (the Telegram bot) and needs no
  third-party.
- Tighten the external probe from 30 min toward ~1–2 min, and have it check a
  liveness field that reflects the loop actually turning, not just the HTTP
  server being up.

**Unblocks.** Confidence to grow past "dozens." Turns silent degradations
(dropped liveness events, a stalled loop, stale backups) into a ping instead of
a support message.

**Dependency.** Most valuable *after* the async-I/O fix (Tier-1 #9), because
loop-lag and desync metrics are the signals that prove that fix is holding.

---

## 3. Timing NFR (≤150 ms) automated e2e test (review 2.3) — effort M

**Problem.** The headline product guarantee — two homes start within ~150 ms —
has **no regression net**. `EffLogDesync` arithmetic is unit-tested, but that
only checks `max−min` of timestamps the test feeds in. There is no automated
multi-node timing test; real desync is only ever observed manually via
`/offset_test` on a live call (`docs/runbook.md`), and the smoke harness
(`agents/skills/run-duet/smoke.sh`) is single-node. A scheduling regression
(e.g. the late-`resume_at`-with-no-catch-up-seek bug, review 2.1) would ship
undetected.

**Proposed approach.**
- A two-fake-node integration test that drives a real `load → ready → resume`
  cycle through the hub with two in-process fake nodes, each with an injected
  clock offset and RTT, and asserts the computed start desync stays within the
  bound. Even a loopback version catches scheduling regressions.
- Extend it with adversarial cases: an RTT spike between `checkAllReady` and
  delivery, a late/negative delay (guards against the 2.1 catch-up-seek gap), a
  poisoned offset (guards 2.2).
- Run it in CI on every PR (it is deterministic — fake clocks, no wall-clock
  sleeps).

**Unblocks.** The ability to change the scheduler, clock-sync, or the node
players without fear; a precondition for fixing 2.1/2.2 confidently. Protects the
one number the product is sold on.

---

## 4. Scaling to ~100–1000 orbits (review 3.x, 1.4, 1.7) — effort L, staged

The current scope is "dozens of orbits," which the design meets comfortably. The
following are what fall over between ~100 and ~1000 orbits. Stage them; do not
do all at once.

**4a. WAL + busy_timeout on the SQLite connection (review 1.7) — S.**
One connection (`MaxOpenConns(1)`) serializes the loop's writes, the hub's
`LookupToken` on every register, the `/media` and `/pair` handlers, and the
retention sweep, with no `journal_mode=WAL` and no `busy_timeout`. A retention
sweep or a big playlist upsert can queue a register behind it and blow the 5 s
register deadline. Open with
`?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate`, allow a small read
pool (WAL permits concurrent readers), keep writes serialized. *This also
formally satisfies Litestream's WAL requirement (`docs/backup-restore.md`).* Low
risk, meaningful headroom — **do this one first and early**, it is nearly free.

**4b. Journal retention for `elements` / `events` (review 3.3) — S.**
These rows are never pruned (only media WAVs are swept); `MarkElementDone` /
`LogEvent` append forever. At scale the DB and single-writer contention grow
without bound. Add a retention sweep (keep 30–90 days) reusing the existing daily
ticker.

**4c. Orbit eviction / lazy load (review 3.2) — M.**
`warmup()` eagerly restores *every* orbit and link at boot with serial DB reads,
and `states`/`groups` never evict. Memory and boot time grow with *total* (not
active) orbit count. Lazy-load orbits on first touch (partly there via
`l.orbit`), drop the eager full restore, and evict idle orbit state on an LRU
with the DB as the backing store.

**4d. The single-writer ceiling (review 1.4) — L, and a decision, not just
code.** `SetMaxOpenConns(1)` + single writable volume + one Coolify app = exactly
one coordinator. SQLite single-writer forbids a second live instance, so there is
no rolling deploy and no active/standby; restarts are stop-the-world. This is
acceptable for the stated scope but is a **hard ceiling**. Past ~1000 busy
orbits, HA needs a different store (Postgres, or Litestream + read replica) or
per-orbit sharding. Treat this as a documented known-limit and a future
architectural decision to schedule *before* it bites, not a bug to fix now.

**Unblocks.** Onboarding strangers and steady growth. Order within the track:
**4a → 4b → 4c**, with **4d** flagged as the eventual wall to plan for
deliberately.

---

## What this plan deliberately leaves for later

- L2 approaches (connected-components → one session per component) — future work,
  needs more tests on the link-lifecycle stale-fallback branches first (review
  3.6).
- Playlist normalization out of a single JSON blob (review 3.4) — only matters if
  playlists get large; revisit with 4c.
- Provider-layer correctness (Odesli false-match gate, proactive availability
  echelon) — audit before the Yandex rollout, tracked with the provider work, not
  here.
