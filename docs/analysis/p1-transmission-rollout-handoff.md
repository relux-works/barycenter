# Phase 1 transmission rollout and downstream handoff

- Date: 2026-07-14
- Task: `TASK-260712-2cdjq8`
- Story: `STORY-260712-25lysg`
- Contract version: `p1-transmission-v1`

This is the stable implementation and operations entry point for phase-one
clip transmission. The [frozen transmission contract](p1-transmission-contract-v1.md)
remains normative; this note records what has shipped, the only supported
mixed-version behavior, the deploy and rollback sequence, and the inputs that
later UI, Telegram, mixer, moderation and Store-compliance work must consume.

This is best-effort engineering evidence. Real applications, packaged installs,
physical Windows/macOS devices, speakers, audible continuity and measured
cross-device timing belong to manual epic `EPIC-260714-th54l3`. They have not
been run or inferred here.

## Shipped boundary

The coordinator now owns one transmission acceptance model, an immutable target
snapshot, a target-scoped media ACL, one durable scheduler per playback domain,
and generation-bound clip commands and receipts. The Go, Windows and Swift wire
codecs share the same additive golden payloads while preserving `play_voice`
and `solo_voice`.

The current macOS and Windows production clients advertise
`media_clip_v1` only after their download/decode/lifecycle client initializes,
in addition to the pre-existing `seamless_adoption_v1`. They deliberately do
not advertise `overlay_mix_v1` or `interrupt_resume_v1` yet. Those capabilities
belong to the later mixer tasks and may be advertised only by a build that can
execute their exact audio behavior. Landing a codec, client hook or scheduler
does not enable a capability by itself.

There is no transmission-wide runtime feature flag in the current coordinator.
Rollout is therefore coordinator first and caller-surface last, with behavior
gated by each authenticated node's advertised capabilities. Operators must not
invent or document a nonexistent flag.

## Final caller contract

| Operation | Frozen behavior |
| --- | --- |
| `POST /v1/transmissions` | Requires a live control credential, a strict 16 KiB JSON object and one actor-scoped `Idempotency-Key`. The caller supplies `media_id`, `audience`, `delivery`, `origin_kind` and optional `include_origin`; it cannot supply identity, `accepted_at`, transmission ID, force, priority or bypass. |
| `GET /v1/transmissions/{id}` | Creator and current source-orbit primary see all target rows. Other current source-orbit actors and exact snapshotted actors see aggregate counts and only their own rows. All other reads collapse to `404 transmission_not_found`. |
| `POST /v1/transmissions/{id}/cancel` | Requires control authority and strict `{}`. Sender cancellation wins only before any target durably enters `playing` or `played`; every node action is generation-bound. Safety actions such as delete, moderation, block and DND may still stop active work. |

Acceptance time and ID are coordinator-owned. The stable FIFO key is
`(accepted_at_ms ASC, transmission_id ASC)`. Exact idempotent replay returns
the stored acceptance; an interrupt confirmation creates no transmission and
gets a fresh acceptance only after an explicit fallback is confirmed.

The supported audiences are `this_pulsar`, `own_barycenter`, `current_air` and
bounded `explicit` selectors. Resolution persists the exact actor, orbit, slot
and binding generation for every target. That immutable snapshot is the only
generic media ACL. Presence, current approach membership, a copied media ID, a
legacy URL or a later replacement binding grants nothing.

Origins are `microphone`, `file`, `telegram` and `builtin`. Microphone excludes
the authenticated origin by default; the other origins include it. An explicit
Boolean overrides the default except that `this_pulsar` cannot be combined with
exclusion. Overlay is limited to exactly 60,000 ms. Longer clips receive the
actionable `overlay_duration_exceeded` error and are never silently changed.

## Delivery and legacy-node window

| Requested delivery | Required current capability | Mixed-version result |
| --- | --- | --- |
| `overlay` | `media_clip_v1` plus `overlay_mix_v1` on every online mandatory target | The whole transmission becomes `after_current` with `mandatory_target_missing_overlay_capability`. No target remains on overlay. |
| `interrupt` | `media_clip_v1`, `interrupt_resume_v1` and, while main is active, `interrupt_resume_ready=true` on every online mandatory target | No transmission is created. The caller receives `409 requires_confirmation` and must explicitly choose an offered overlay or after-current fallback. |
| `after_current` | Existing `play_voice` compatibility or the equivalent main-session bridge | One persistent legacy Session element is created for the exact target set. |

The **legacy-node window** begins when the new coordinator is deployed while
any eligible target still runs a build without the requested capability. It
ends only when all target builds have reconnected and advertised the exact
capabilities they can execute. A reconnect replaces, rather than unions, the
previous capability set. Offline, DND-suppressed and blocked rows remain in
history but do not force a capability downgrade because they are not mandatory
targets at the acceptance decision.

There is never a per-target protocol split. A confirmed overlay remains
overlay and cannot silently downgrade later. An accepted interrupt that loses
capability later fails the affected target with
`failed/interrupt_capability_lost`; the scheduler never invents a fallback.

## Final receipt and presentation model

The target lifecycle is:

```text
accepted -> preparing -> ready -> scheduled -> playing -> played
                 |         |          |           |
                 +---------+----------+-----------+-> cancelling -> cancelled

terminal without playback:
missed_offline | missed_dnd | missed_not_ready | blocked |
failed | cancelled | expired
```

Only `media_ended(reason=completed)` proves `played`; prepared, ready,
scheduled or started is not delivery success. Target reason codes are closed:

| Target status | Exact reason codes |
| --- | --- |
| `played` | `completed` |
| `missed_offline` | `offline_at_acceptance`, `offline_before_prepare`, `offline_before_start` |
| `missed_dnd` | `local_dnd`, `orbit_dnd` |
| `missed_not_ready` | `prepare_deadline` |
| `blocked` | `actor_blocked`, `orbit_blocked` |
| `failed` | `media_download_failed`, `media_auth_failed`, `media_expired`, `hash_mismatch`, `decode_failed`, `duration_mismatch`, `clock_unsynchronized`, `stale_play`, `device_unavailable`, `audio_graph_failed`, `connection_lost`, `capability_lost`, `interrupt_capability_lost`, `cancel_unacknowledged`, `internal_error` |
| `cancelled` | `sender_cancelled`, `media_deleted`, `media_expired`, `moderation_disabled`, `approach_left`, `approach_apart`, `target_revoked`, `dnd_enabled`, `sender_blocked`, `coordinator_restarted` |
| `expired` | `delivery_expired` |

The aggregate status is one of `accepted`, `preparing`, `scheduled`, `playing`,
`cancelling`, `played`, `partial`, `failed`, `cancelled` or `expired`. While
work is nonterminal the precedence is playing, cancelling, scheduled,
preparing, then accepted. When every row is terminal, all played means
`played`; a mix containing a played row means `partial`; a committed
transmission cancellation or delivery expiry wins its matching aggregate; all
other no-success sets mean `failed`.

The only terminal aggregate reasons are `completed`, `partial_delivery`,
`sender_cancelled`, `media_deleted`, `media_expired`, `moderation_disabled`,
`approach_left`, `approach_apart`, `coordinator_restarted`,
`delivery_expired`, `no_eligible_targets`, `no_ready_targets` and
`all_targets_failed`, in the combinations constrained by the normative matrix.
UI, history and Telegram may localize labels, but must retain the exact machine
status and reason and must never render a missed, blocked, failed, cancelled or
expired row as played.

The WebSocket lifecycle is `prepare_media` then `media_ready`, followed by one
`play_media_at` for every ready target and exactly one terminal
`media_ended`, `media_failed` or `media_cancelled` receipt. `cancel_media` is
the generation-bound disarm/fade-stop carrier. The coordinator uses a strict
three-second prepare barrier and one start time:

```text
T = decision_now + max(2 * fresh_max_RTT + 250 ms, 500 ms)
start_deadline = T + 100 ms
```

`after_current` remains on the Session/legacy voice path and does not receive
`play_media_at` in phase one. The exact fields and conditional validation live
in the [wire contract](p1-clip-transmission-wire-contract.md) and executable
golden set.

## Presence, DND and block handoff

| Concern | Downstream rule |
| --- | --- |
| Presence | Render only the authorized orbit/slot projection: online, last-seen coordinator time, output state, playback state, effective DND, capability labels and interrupt readiness. Offline starts after 12 seconds and forces output `unavailable`, playback `unknown` and interrupt readiness false; last-known capabilities are display-only. Never expose actor, token, socket, IP, host, device, process, capture, level, path or URL data. |
| DND | Modes are `allow_all`, `messages_only` and `muted_until`. Effective DND is the stricter local/orbit layer. `messages_only` permits user clips but suppresses built-ins/automation. Only exact authenticated `this_pulsar` local intent bypasses DND for itself. No remote sender, Telegram action or moderator can loosen it. |
| Block | Recipient-owned actor/orbit blocks are checked before DND, presence and capability. They grant no byte ACL. A later block terminates non-started work as blocked and active work as `cancelled/sender_blocked`; removal never resurrects a terminal transmission. |
| Receipt visibility | Creator and source primary receive all rows; another source or snapshotted actor receives aggregate counts and only its bound rows. Hidden block scope, actor IDs and capability diagnostics remain undisclosed. |

## Required rollout order

1. Record the coordinator and node artifact revisions and checksums. Take a
   SQLite-safe backup using the existing runbook and retain the immediate
   predecessor artifact. Do not remove unknown additive tables during backup
   or restore rehearsal.
2. Deploy the new coordinator **first**. Startup installs additive transmission
   and scheduler tables. Keep every new create surface undiscoverable or
   disabled at its owning UI, Telegram dispatcher and local-action boundary;
   existing legacy playback continues while GET/receipt internals settle.
3. Verify coordinator health and deterministic schema compatibility. Confirm
   that legacy nodes reconnect and that their exact capability set replaces
   stale state. Do not infer speaker or hardware behavior from health or CI.
4. Deploy macOS and Windows clip-client builds. A successful client
   initialization may advertise `media_clip_v1`; it still withholds overlay and
   interrupt capabilities until the respective mixer implementation lands.
5. Deploy mixer-capable builds platform by platform. Advertise
   `overlay_mix_v1` and `interrupt_resume_v1` independently, only when the
   corresponding executable path and regression suite are present. Mixed
   fleets follow the whole-downgrade/challenge table above.
6. Expose desktop and Telegram create surfaces last, to a bounded cohort.
   Every surface must display requested versus effective delivery, a visible
   downgrade or confirmation choice, exact aggregate/target outcomes and
   `can_cancel`; it must not claim that acceptance or start means playback.
7. Monitor nonterminal transmission rows, receipt failures, confirmation and
   downgrade rates, scheduler watchdog outcomes, media lifecycle backlog and
   reconnect churn. Continue ordinary best-effort engineering rollout only;
   physical playback and timing claims remain gated by the manual epic.

## Drain-before-rollback procedure

Because there is no single runtime transmission flag, rollback starts by
withdrawing every creator at its owning surface: reject new HTTP creates,
pause Telegram media acceptance and disable local create actions while keeping
status/cancel processing available.

1. Cancel eligible queued/prepared transmissions through the normal API or
   audited moderation/lifecycle path. Let already-playing work finish or use
   only its defined safety cancellation cause.
2. Wait for every generation-bound disarm/cancel acknowledgement and bounded
   watchdog. Read-only inspection must reach zero:

   ```sql
   SELECT COUNT(*) AS nonterminal_transmissions
   FROM transmissions
   WHERE completed_at = 0;
   ```

   Do not manually rewrite transmission, target or scheduler status rows to
   manufacture a drain.
3. Stop the current coordinator so there is one SQLite writer. Take another
   SQLite-safe backup/checkpoint and record the drained artifact revision.
4. Deploy the recorded immediate predecessor. Leave all additive
   `transmissions`, target, idempotency, confirmation and scheduler tables in
   place. Exact predecessor tests prove they are ignored safely; dropping or
   down-migrating them destroys roll-forward state.
5. Keep new create surfaces withdrawn. Legacy `play_voice`/`solo_voice`
   remains the supported mixed-version behavior. Nodes must not synthesize
   receipts for commands they did not receive.

If the drain cannot reach zero, stop the rollback and restore the current
coordinator; do not improvise SQL mutation. For roll-forward, deploy the
current coordinator while surfaces remain withdrawn, let additive schema
installation and reconciliation finish, verify preserved transmission/target
ACL/scheduler rows, reconnect nodes, and only then reopen callers in the same
coordinator-first order.

## Exact downstream ownership

| Consumer | Contract handed over; no product decision remains here |
| --- | --- |
| Overlay/interrupt mixer (`STORY-260712-fes2jj`) | Implement one render-safe controller, exact duck/attack/release or fade/resume semantics, and advertise each capability only after it is executable. Preserve generation, T/deadline, one-domain serialization and exact terminal receipts; never invent fallback. |
| Telegram/history/presence (`STORY-260712-34kbkn`) | Call the common service with verified Telegram context and `origin_kind=telegram`; never supply identity/acceptance/bypass. Present requested/effective delivery, explicit interrupt choice, exact statuses/reasons, privacy-bounded presence and authorized target rows. Replay is a new acceptance, not resurrection. |
| Main UI/capture (`STORY-260712-2e36uz`) | Use strict create/status/cancel shapes and retain local drafts across retry. Show downgrade, confirmation, target counts, `can_cancel`, DND/block effects and honest terminal results. Do not call acceptance, ready or started a successful play. |
| Policy/moderation (`STORY-260712-1tgryz`) | Reuse canonical block, media delete/expiry and `moderation_disabled` cancellation paths. Moderation can tighten or stop delivery through audited authority but cannot bypass recipient DND/block or broaden receipt/media visibility. |
| Store compliance (`STORY-260712-1i0doc`) | Describe capability-gated mixed fleets, visible downgrade/confirmation, privacy projection, content-free receipt retention and rollback. Do not claim audible continuity, platform behavior, physical skew or real-device success until the separate manual evidence is accepted. |
| P2 Air and targets/inbox (`STORY-260712-3v14m9`, `STORY-260712-ob1tx2`) | Extend the immutable snapshot and receipt model without reinterpreting phase-one enums or using current room/inbox visibility as media authority. A replay or new target set creates a new transmission acceptance. |

## Contract, diagrams and evidence index

- [Normative phase-one transmission contract](p1-transmission-contract-v1.md)
- [HTTP resolution and authorization outcome](p1-transmission-http-resolution.md)
- [Clip WebSocket wire contract](p1-clip-transmission-wire-contract.md)
- [Immutable target snapshot and media ACL](p1-transmission-store-target-snapshots.md)
- [Protocol implementation reference](../protocol.md)
- [Component diagram](../diagrams/p1-transmission-protocol-components.puml)
- [Scheduler sequence diagram](../diagrams/p1-transmission-scheduler-sequence.puml)
- [macOS client hook outcome](../../.task-board/.resources/TASK-260712-26ip33/macos-transmission-client-hooks-outcome.md)
- [Windows client hook outcome](../../.task-board/.resources/TASK-260712-2bbz13/windows-transmission-client-hooks-outcome.md)
- [Scheduler outcome](../../.task-board/.resources/TASK-260712-31vvjt/overlay-controller-scheduler-outcome.md)
- [Accepted deterministic regression evidence](../../.task-board/.resources/TASK-260712-2qc27p/transmission-regression-evidence.md)
- [Deferred manual-test plan](../../.planning/260714_045154_epic-260714-th54l3.md)

The regression evidence pins strict HTTP input, all valid terminal
status/reason pairs, FIFO and max-RTT timing, capability rechecks, cancellation,
restart, exact predecessor rollback and the legacy whole-downgrade path. It
explicitly leaves audible and physical-hardware observations unpassed.
