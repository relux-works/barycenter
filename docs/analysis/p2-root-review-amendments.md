# Phase 2 decomposition — root review amendments

Date: 2026-07-12. These amendments and the current task cards supersede any
conflicting wording in the initial Phase 2 decomposition artifacts.

## Codec and transport gate

- Phase 2 starts only after the Phase 1 A1-A8/Store gate task
  `TASK-260712-1xik11`; codec, Air and target foundation tasks carry that hard
  dependency.
- The spike compares complete Windows+macOS combinations. Native macOS
  AVFoundation/AudioToolbox is an explicit candidate alongside Media
  Foundation, pure-Go, and bundled signed decoders.
- Hard gates are fixed before experiments: start p95 <=5 s, seek-to-audio p95
  <=3 s, scheduled skew p95 <=100 ms, RSS <=200 MiB for B1, and resource use
  independent of duration through the two-hour maximum.
- The shared corpus includes exact MP3/AAC/Opus containers, CBR/VBR and hostile
  inputs and all Windows-Windows, Windows-macOS, and macOS-macOS pairings.
- Range transport includes target authorization on every request, RFC range and
  conditional behavior, integrity/chunk metadata, VBR seek mapping, private
  caching, revocation, amplification limits, and bounded app-private disk.
- License review is version- and dependency-specific, with authoritative
  sources, SBOM, notices, patents/legal escalation, signatures, Store and
  notarization posture, CVE/update ownership, and no first-run executable
  download. The ADR must publish `no-go` if no complete combination passes.

## Streamed main program

- Long tracks never receive the Phase 1 speech processing chain by accident.
  Variants and seek metadata come exactly from the approved ADR; hostile
  transcoding remains sandboxed and atomically published.
- Main playback is provider-neutral rather than a second Spotify-shaped branch.
  Progress and restart anchors are audible, seek uses generations and a fresh
  readiness barrier, and ended occurs only after decoder/output drain.
- Windows and macOS keep network, disk, decode, allocation, waits, and blocking
  locks outside render callbacks. Cache is bounded and purged/invalidated on
  canonical delete/disable.
- Storage, processing, and egress counters plus quotas are implemented and
  reconciled across crash, retry, range refill, delete, and retention; operator
  views consume those counters rather than creating a parallel accounting path.
- Shared and per-platform UI tasks cover long-file policy consent, durable
  resumable draft, processing versus playback progress, Air/target selection,
  queue/replace, pause/seek/resume, quota/rebuffer errors, and accessibility.

## Air rooms

- Saved Air membership is distinct from the one-active-Air runtime pointer.
  Parked rooms are lazy and have no automatic GC in Phase 2.
- Invites are hashed, single-use, rate-limited, redacted, and require primary
  confirmation by the joining Barycenter. Lifecycle and policy mutations are
  actor/role authorized and audited across app, secure Telegram callbacks, and
  legacy aliases.
- Link backfill has a deterministic mapping and explicit authority cutover.
  A link runtime and Air runtime can never deliver concurrently; rollback
  preserves Phase 2 rows and legacy service without resurrecting duplicates.
- Exactly one active Air owns one deduplicated peer union and queue. A joining
  member may catch up the current main track after buffer readiness but never an
  old overlay. Leave stops/fades only leavers and restores personal state.
- Windows/macOS manage multiple saved Airs and one active Air; disruptive
  switch/leave/dissolve requires confirmation. Telegram has full Air 2..N
  lifecycle parity, not only `/approach` and `/apart`.

## Explicit targets, inbox, and rights

- Phase 2 extends Phase 1 target snapshots, history, secure callbacks,
  moderation, and delete; it does not create a second ACL/report stack.
- Opaque server-authorized selectors resolve to exact deduplicated nodes. The
  blocker contract must freeze targeted-track behavior and mixed-version
  policy. N-recipient personal delivery never broadcasts as fallback.
- Inbox owns one eligible item per missed target, has TTL bounded by media
  expiry, stable pagination and replay lineage, and never starts from list or
  reconnect. Replay is a new explicit transmission with new accepted time and
  newly resolved targets. New members do not inherit old items.
- File uploads use versioned actor-scoped policy consent and a rights reminder.
  A report protects the reporter but cannot be an unauthenticated global
  censorship primitive; canonical sender/moderator delete or actor/orbit
  disable performs global fetch/replay revocation.
- Shared plus separate Windows/macOS UI and secure Telegram tasks use identical
  target, consent, inbox, queue/replace, receipt, and moderation semantics.
- Story ordering is Air → explicit targets/inbox → streamed tracks. The combined
  Telegram N-target/track adapter `TASK-260712-wt2n7m` lives with streamed
  tracks and is consumed by Phase 2 acceptance; targets no longer depend back
  on an unfinished track story.

## Acceptance and review

- The lab contract freezes samples, clocks, fixture/build hashes, real topology,
  participant privacy, resource thresholds, migration fixtures, and seven
  consecutive 24-hour beta windows.
- Rollback quiesces Phase 2 and disables flags before a previous binary starts;
  it proves no dual runtime and preserves Phase 2 rows for later roll-forward.
- Four non-implementing reviewers independently cover codec/supply-chain,
  target/range/rights security, Air migration/concurrency, and streaming
  performance/realtime behavior. Critical/high findings require fixes and
  re-review.
- Root personally reviews the complete diff and reruns Phase 1+2 regressions
  before final B1-B7 or beta. Any critical beta incident or unapproved build
  change resets the seven-day gate.
