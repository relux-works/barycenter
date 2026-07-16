# P2 streamed-track rollout, limits and operational handoff

- Date: 2026-07-16
- Task: `TASK-260712-2ubzyf`
- Contract: `p2-streamed-track-rollout-handoff.v1`
- Machine-readable companion:
  [`acceptance/streamed-track-rollout-handoff-v1.json`](../../acceptance/streamed-track-rollout-handoff-v1.json)

This is the final repository-engineering handoff for the streamed user-audio
story. It freezes the selected variant result, bounded cache and transport,
quota defaults, rollout/rollback order, operator views and seams to Air,
targets/inbox and Phase 2 acceptance.

It is not a production enablement record. The accepted codec/player ADR is a
no-go, no production profile exists, neither desktop build advertises
`stream_track_v1`, and the `streamed_tracks` runtime feature flag is
intentionally absent. Real application, audible, accessibility, hardware,
mixed-fleet, rollback and beta evidence remains in `EPIC-260714-th54l3`.

## Production decision and feature-flag assumptions

The chosen production variant matrix is empty. MP3/MP3, AAC-LC/fast-start M4A,
AAC-LC/ADTS and Opus/Ogg remain schema- and test-only pairs. The original
upload is never decoder input, clients never negotiate a codec, and only one
server-pinned immutable ready variant may eventually be selected.

This is enforced below the UI. `stream_variant_policy` stores
`p2-codec-player-adr-handoff.v1` and its current schema `CHECK` permits only
`production_selection_enabled=0` with an empty profile. The production encoder
and decoder registries are empty. Queue/replace adapters fail closed and nodes
reject a stream command received despite not advertising the capability.

The specification names `streamed_tracks`, but the current runtime does not
implement a switch that could safely bypass those guards. A replacement ADR
must first select one exact cross-platform combination, close packaging,
license and Store obligations, add explicit registries and land an additive
policy-schema revision. Only that reviewed change may add a coordinator-owned,
default-off, per-orbit allowlist. A hidden caller control, environment variable,
database edit or client capability string is never feature-flag authority.

## Frozen variant, transport and cache limits

| Boundary | Frozen value |
| --- | ---: |
| Upload input | 500 MiB |
| Maximum admitted duration | 7,200,000 ms (two hours) |
| Verified chunk / range response / network read | 1 MiB |
| Installation-private cache | 512 MiB |
| One variant namespace | 64 MiB |
| Simultaneously pinned chunks | 128 MiB |
| Decoder PCM ring | 1 MiB |
| Seek-map point spacing | at most 10,000 ms |
| Initial buffer barrier | 2,000 ms |
| Track start p95 gate | 5,000 ms |
| Seek-to-audio p95 gate | 3,000 ms |
| Start skew p95 gate | 100 ms |

The cache ceilings are duration-independent. Keys are installation-secret
HMACs over authorization namespace, variant, ETag and chunk index. Writes are
atomic, restart reconciles metadata and files, and unpinned LRU eviction obeys
both global and per-variant ceilings. Integrity is SHA-256 before cache
visibility or decode. A mixed ETag invalidates the namespace and requires
manifest resolution; bytes from two versions are never combined.

Every GET/HEAD to `/v1/media/{media_id}/variants/{variant_id}` reauthorizes the
node credential against the exact immutable target snapshot plus current
binding, membership, block, report, media, owner and variant state. URLs,
manifests, ETags, current Air membership and cache possession grant nothing.
The production response is private/no-store and foreign, deleted, disabled,
revoked and unknown objects share one `404` surface.

## Quota defaults and operator metrics

The following binary-byte defaults are conservative engineering gates until
`TASK-260712-2pnc5a` performs the seven-day manual beta calibration:

| Limit | Actor | Orbit |
| --- | ---: | ---: |
| Upload starts / 24 h | 100 | 500 |
| Input bytes / 24 h | 5 GiB | 25 GiB |
| Canonical retained bytes | 10 GiB | 50 GiB |
| Processing temp reservation | 2 GiB | 8 GiB |
| Concurrent processing jobs | 2 | 8 |
| Total retained track bytes | 20 GiB | 100 GiB |
| Actual + active-reserved egress / 24 h | 100 GiB | 500 GiB |

One playback generation reserves at most twice the immutable variant size.
Quota changes are revision-conditional audited operator decisions. Lowering a
limit blocks new work but never corrupts an already admitted playback.
Processing and egress leases stale after 30 minutes; startup and the five-minute
loop release stale reservations and increment crash-release counters.

Public `/healthz` exposes only `ready`, `saturated` and
`last_reconciled_at`. Exact values require a live moderation operator with the
proper capability:

- `GET /v1/moderation/stream-accounting` exposes storage, processing, range,
  egress, crash-release, quota-rejection and saturated-scope metrics;
- `POST /v1/moderation/stream-accounting/policies` applies a
  revision-conditional override with bounded reason and operator audit;
- `GET /v1/moderation/stream-accounting/policies/audit` reads immutable changes.

`ready=false`, unexplained `saturated=true`, unexplained crash-release growth,
or an unreconciled quota/egress mismatch blocks cohort expansion. The later
observability task must consume these authorities, not create parallel
eventually-consistent counters.

## Required rollout order

The current artifact may execute stages 1-4 only. Stages 5-8 are an explicit
future runbook, not permission to activate the current build.

1. **Freeze, back up and record.** Record coordinator/Windows/macOS/Telegram
   artifact hashes, codec ADR and content-policy hashes; take a SQLite-safe
   backup and retain the tested predecessor.
2. **Deploy coordinator schema dark.** Coordinator goes first. Install the
   additive stream, processing, accounting, playback and queue tables while
   production selection stays disabled and queue/replace callers stay closed.
3. **Reconcile and observe dark.** Complete integrity/foreign-key checks, run
   lease reconciliation, verify redacted health, authenticated operator views,
   Phase 1 traffic and exact-predecessor preservation.
4. **Deploy clients and adapters dark.** Deploy candidate-neutral cache/player
   seams and the shared Windows/macOS/Telegram projections. Advertise no
   `stream_track_v1`; render unsupported states; preserve clip and Spotify.
5. **Pass the replacement-ADR/runtime gate.** Select an exact codec/player,
   implement production encoder/decoder registries, revise the policy schema
   additively, close signing/notarization/license/Store obligations and rerun
   the complete matrix. The current no-go blocks this stage.
6. **Enable one internal orbit.** Add one coordinator-owned allowlist entry,
   select the reviewed server profile, require exact capabilities from newly
   reconnected nodes, and run the real B1 platform and audible gates. No
   current artifact can perform this step.
7. **Expand bounded cohorts.** Expand only after telemetry and incident review.
   Preserve sender-selected mixed-version policy, canonical revocation and the
   immediate flag-off/drain path.
8. **Store/public promotion.** Require the real packaged platform matrix,
   production-shaped rollback rehearsal, seven-day quota calibration and the
   accepted Phase 2 promotion packet.

## Mixed-version and cross-story behavior

The sender chooses `require_all` or `supported_only_with_receipts` before the
immutable target snapshot is queued. The first rejects atomically if any
frozen target lacks `stream_track_v1`. The second queues capable targets and
creates visible terminal `unsupported` receipts for the rest. There is no
clip, Spotify, broadcast or plaintext fallback and no late autoplay after an
upgrade. Reconnect replaces the capability set and affects only a new sender
decision and snapshot.

Air remains the membership/lifecycle authority. A join during a current track
may catch up only the capable joining home at the current audible position; a
leave cancels only the leaver. Existing participants continue. Air never owns
range authorization, variant selection or cache state.

Targets/inbox remains the delivery, receipt, replay, rights and moderation
authority. A range/refill uses the exact accepted target row. Inbox visibility
does not grant bytes, and replay is a new explicit authorization and target
snapshot. Telegram uses the common service and opaque callbacks; it owns no
parallel queue, cache, ACL or rights state.

## Delete, report, disable and cache revocation

| Action | Future fetch/cache behavior |
| --- | --- |
| Plain user report | Reporter-local hide and no-refill only; another accepted target is not globally revoked. |
| Sender or moderator delete | Every future open is uniform `404`; matching generation-bound work is cancelled; unpinned bytes are removed and pinned chunks are tombstoned. |
| Variant revoke | Terminal `404` for every future open and durable no-refill. |
| Owner actor/orbit disable | Terminal `404` for every future open and durable no-refill. |

An immutable file descriptor already opened inside the authorization
transaction may finish only its current bounded response (at most one verified
1 MiB chunk). It grants no later open. Playback cancellation is a separate
generation-bound scheduler action. Historical actual egress and terminal
receipts remain; current retained-byte projections drop deleted/expired/revoked
objects immediately.

## Drain-before-rollback and roll-forward

Rollback starts by turning off the future coordinator-owned flag and
withdrawing new upload exposure, queue, replace, streamed replay and Telegram
track callbacks. History/receipt reads and safety operations—delete, report,
block, audited disable and generation-bound cancel—remain available.

1. Finish or canonically cancel active generations and wait for bounded
   acknowledgements/watchdogs. Never edit queue, target, playback or accounting
   rows with ad-hoc SQL.
2. Stop the coordinator so SQLite has one writer; checkpoint, back up and
   record drained artifact/database hashes.
3. Deploy only the retained tested predecessor
   `06a06c099ed5b4f37f5e2dd3648772ffd041dfd9`. Do not down-migrate or drop
   additive stream, accounting, target, inbox, consent or moderation tables.
4. Keep new track callers withdrawn. The predecessor may ignore unknown rows
   but must not reinterpret or delete them.

Roll-forward keeps callers closed, redeploys the current coordinator,
reconciles additive rows, checks target ACL, accounting and generations, then
repeats the ordered gates. If any preservation or reconciliation check fails,
restore/forward-fix with the current coordinator; never improvise destructive
migration.

## Acceptance and manual boundary

- `TASK-260712-14rxuk` maps this handoff into the Phase 2 evidence contract
  without claiming rollout execution.
- `TASK-260712-qi81vf` builds views from the exact existing accounting fields.
- `TASK-260712-1kfnpu` reviews no-go preservation, rollback, target security and
  the residual manual boundary.
- `TASK-260712-1fpb9q`, `TASK-260712-21kz3b`, `TASK-260712-2bdi4a`,
  `TASK-260712-3qybi2`, `TASK-260712-3u5cdn` and `TASK-260712-2pnc5a` remain
  manual-required under `EPIC-260714-th54l3`.
- `TASK-260712-3a0cf9` cannot produce a promotion packet until the applicable
  engineering reviews and manual gates are explicit and accepted.

Validate this repository handoff with:

```sh
python3 scripts/validate_streamed_track_rollout_handoff.py
python3 -m unittest scripts/acceptance/test_streamed_track_rollout.py
```

Passing those commands proves documentation/implementation consistency only.
It does not advance the current maximum rollout stage beyond dark deployment.
