# Phase 2 targets and inbox rollout handoff

- Date: 2026-07-16
- Task: `TASK-260712-20cuna`
- Contract: `p2-targets-inbox-rollout-handoff.v1`
- Normative source: `p2-targets-inbox-parity.v1`

This is the final engineering entry point for the explicit-target, inbox and
rights story. It records the implemented API and authority boundary, the only
supported coordinator-first deployment and rollback order, and the assumptions
that streamed tracks, Phase 2 acceptance and future E2EE work must consume.
The machine-readable companion is
[`acceptance/targets-inbox-rollout-handoff-v1.json`](../../acceptance/targets-inbox-rollout-handoff-v1.json).

This document does not say that a production rollout, packaged mixed fleet,
physical device, accessibility reader, audible replay or real network denial
has passed. Those results remain `manual-required` in
`EPIC-260714-th54l3`, specifically `TASK-260712-3u5cdn` and
`TASK-260712-3qybi2`.

## Final API, receipt and rights contract

There is one delivery authority. `POST /v1/transmissions` authenticates a
control credential, derives `ActorContext`, resolves at most 64 opaque target
references inside the caller's own Barycenter or current Air, deduplicates
before origin filtering, checks the current content-policy grant and commits
one immutable target snapshot atomically. Knowledge of a media, transmission,
history or inbox handle and current Air membership are not grants.

| Surface | Final behavior |
| --- | --- |
| `POST /v1/transmissions` | Coordinator-owned acceptance. Explicit selectors are opaque, actor/scope/domain/binding-bound and expiring. A stale or foreign selector is nonexistent. Any mandatory online unsupported target rejects the whole create with `422 unsupported_targets`; no partial rows or broadcast fallback are allowed. |
| `GET /v1/inbox` | Exact current recipient binding only. Keyset order is `(created_at DESC, inbox_id DESC)` with a frozen upper bound. The random `ic_` cursor is digest-only and binds actor, credential scope, binding generation, view, limit and page keys. Reads cannot schedule, queue or play. |
| `GET /v1/inbox/{ib_}` | Exact current target binding and current media/rights state are rechecked. Foreign, replaced, disabled, deleted and forged objects use the uniform nonexistence surface. |
| `DELETE /v1/inbox/{ib_}` | Recipient-local idempotent dismissal. It does not delete media, change the immutable receipt or affect another target. |
| `POST /v1/inbox/{ib_}/replays` | Requires an explicit current user action, idempotency and fresh actor/binding/media/policy authorization. It creates a new transmission and immutable target row, records root/depth lineage, then consumes the inbox item atomically. It never resurrects or edits the original receipt. |
| `GET /v1/history/{hi_}/receipts` | Sender gets safe labels for the immutable target set; a target gets only its exact row. The `rc_` cursor is independent and digest-only. Raw actor/orbit/slot/binding IDs are never returned. |
| `POST /v1/history/{hi_}/actions/report` | Requires the reporter's exact persisted target evidence. Immediate protection is reporter-local: deny that actor fetch/replay/future delivery and stop only that actor's matching work. A report count cannot globally quarantine, delete or disable. |
| `POST /v1/history/{hi_}/actions/delete` | Reuses the canonical sender media lifecycle. Fetch is revoked before asynchronous cleanup, pending/active work is cancelled through normal generation-bound paths, inbox replay disappears and terminal receipts remain. |

One eligible terminal target receipt creates at most one inbox row in the same
SQLite transaction. Eligibility is limited to the frozen offline, DND,
prepare-deadline, connection-loss, device-unavailable and audio-graph-failure
pairs. The expiry is the earliest of 30 days after receipt, media expiry and
policy retention. Expired rows may remain terminal-readable but have no replay
authority. Replacement bindings and later Air members inherit neither the row
nor media bytes.

Action hints are presentation only. Every replay, dismiss, delete, report,
block and moderation operation reauthorizes through its canonical service.
Reviewed quarantine, media delete and actor/orbit disable are audited operator
decisions; they revoke fetch, future delivery and replay without rewriting
historical target truth. Windows, macOS and Telegram consume the same portable
states, commands and canonical outcomes. Telegram stores no parallel queue,
inbox, rights or moderation state.

The normative field/enumeration details remain in the
[frozen contract](p2-targets-inbox-contract-v1.md), the
[store and ACL result](p2-target-inbox-store-acl.md), the
[inbox/history API result](p2-inbox-history-api-pagination.md), the
[rights enforcement result](p2-rights-report-disable-enforcement.md) and the
[portable presentation model](p2-pulsar-targets-inbox-presentation-model.md).
The [component](../diagrams/p2-targets-inbox-parity-components.puml) and
[sequence](../diagrams/p2-targets-inbox-parity-sequence.puml) diagrams show the
same authority graph and explicit replay flow.

## Required rollout order

There is no targets/inbox-wide coordinator feature flag in the implemented
runtime. The safety boundary is coordinator first and caller surfaces last.
Operators must not invent a flag or treat a hidden button as server-side
authorization.

1. **Freeze and back up.** Record coordinator, Windows, macOS and Telegram
   artifact revisions and hashes; record the current policy version/hash; take
   a SQLite-safe backup; retain the immediate predecessor coordinator and
   current rollback artifact. Do not remove unknown additive tables.
2. **Deploy the coordinator dark.** Withdraw explicit-target creates and inbox
   mutations at every caller surface before changing the server artifact.
   Keep Phase 1 traffic and safety operations available. Startup installs
   additive transmission/inbox/cursor/content-policy/moderation state in
   transactions and must complete `foreign_key_check` before commit.
3. **Reconcile before exposure.** Let startup freeze missing target capability
   hashes/resolution times, install scheduler companions and backfill only
   eligible historical receipts. Verify Phase 1 create/play/history, exact
   previous-head compatibility, uniform non-target nonexistence, read-only
   inbox behavior and zero reconnect autoplay. A health endpoint is not audio
   or physical-device evidence.
4. **Deploy consumers dark.** Deploy Windows and macOS builds that consume
   `pulsar.targets-inbox-presentation.v1`, then the Telegram adapter over the
   common service. Reconnect replaces rather than unions capability sets.
   Unknown fields and enums remain visibly unsupported. Do not expose a sender
   merely because a model can render the response.
5. **Expose clips to a bounded cohort.** Enable current-Air and explicit clip
   selection at the owning caller surfaces. Monitor atomic
   `unsupported_targets`, selector/cursor failures, replay outcomes, inbox
   expiry, report-local revocation, canonical delete/disable and reconnect
   churn. Preserve explicit replay and no autoplay/autoqueue.
6. **Extend to streamed tracks only after `STORY-260712-2ori1t`.** Complete
   `TASK-260712-1n5fks`, `TASK-260712-3lf8r0`, players, coordinator queue or
   replace runtime and `TASK-260712-wt2n7m` before advertising
   `audio_track_v1`, `queue_replace_v1` or `stream_variant_v1`. Reuse this
   story's target, inbox, lineage and revocation authorities, then rerun B5-B7
   delta regressions. Until then every targeted track stays visibly
   unsupported.
7. **Run manual gates before promotion.** `TASK-260712-3u5cdn` executes real
   packaged B5-B7 mixed-fleet/rights acceptance. `TASK-260712-3qybi2`
   rehearses backup, upgrade, drain, predecessor rollback and roll-forward
   with exact artifact/schema/data hashes. Documentation and CI do not satisfy
   either task.

## Mixed-version window

The mixed-version window starts when the new coordinator is deployed while any
eligible target or caller still lacks the exact executable capability. It ends
only when current builds reconnect and replace their advertised capability
sets and the applicable manual gate is accepted.

- A Phase 1 clip remains allowed only when every mandatory online target can
  execute its requested Phase 1 delivery. Existing whole-transmission
  downgrade/confirmation rules remain intact.
- Before the streamed-track story lands, a targeted track is visibly
  unsupported. No transmission, inbox row, queue action, broadcast or clip
  fallback may be synthesized.
- When track runtime exists, any mandatory online target missing an exact
  requirement causes one atomic `422 unsupported_targets` with opaque target
  references and sorted missing capabilities. There is no partial create.
- An offline target with unknown current capability is frozen into the
  acceptance and may produce an eligible missed receipt. Reconnect cannot
  autoplay or autoqueue it; replay is a new explicit acceptance.
- Capability loss after prepare fails only the affected exact target with the
  existing receipt model. Peers do not downgrade and a new member does not join
  the immutable snapshot.
- Unknown future values are retained and rendered unsupported, never coerced
  into a known action.
- Future E2EE work may fail or request explicit exclusion/confirmation under
  its reviewed contract, but it may never silently fall back to plaintext.

## Drain-before-rollback and roll-forward

Rollback begins at caller surfaces. Withdraw explicit-target creates, inbox
replay, new Phase 2 Telegram callbacks and any future streamed-track queue or
replace action. Keep read/status and safety mutations available: dismissal,
sender delete, report, block, audited moderation disable and eligible
transmission cancel.

1. Finish nonterminal work or terminate it only through canonical cancellation,
   delete, expiry, block, DND or moderation paths. Wait for generation-bound
   acknowledgements and bounded watchdogs. Do not edit target, inbox, lineage
   or lifecycle rows with ad-hoc SQL to manufacture a drain.
2. Stop the coordinator so there is one SQLite writer. Take a fresh safe
   checkpoint/backup and record the drained artifact and database hashes.
3. Deploy only the retained tested predecessor. Do not down-migrate or drop
   `transmission_target_references`, `transmission_targets`,
   `transmission_inbox_items`, `transmission_replay_lineage`, cursor tables,
   `content_policy_acceptances` or `moderation_reports`. The predecessor may ignore
   additive data; it must not delete or reinterpret it.
4. Keep new callers withdrawn while the predecessor serves its supported Phase
   1 boundary. Do not mint new Phase 2 callbacks or synthesize receipts for
   commands it never issued.

The pinned rollback suite currently executes exact predecessors
`0c1e1946ff692aa553c19ca6bf7328150d1a24b8` and
`2aa97c2d08cb93b110200ae159fd43265410ff5a`. It proves repository compatibility,
not a real deployment rehearsal. If drain, startup or compatibility fails,
stop and restore the current coordinator; never improvise a destructive
migration.

Roll-forward uses the same coordinator-first sequence: callers stay withdrawn,
the current coordinator installs/reconciles additive rows, target ACL and
replay lineage are verified, current clients reconnect with replacement
capabilities, then caller surfaces reopen in bounded order. Preserved Phase 2
rows must become visible only to their exact still-authorized bindings.

## Downstream ownership

| Consumer | Required use of this handoff |
| --- | --- |
| `TASK-260712-1n5fks` | Extend media/variant and playback persistence without adding another target, inbox, history or moderation authority. Old clip and Spotify rows remain readable. |
| `TASK-260712-3lf8r0` | Authorize every track range and cache refill against the immutable target snapshot plus current binding/media/block/report/disable state. A URL, ETag, cache key or current Air membership grants nothing. |
| `TASK-260712-wt2n7m` | Route Telegram target, inbox and track actions through the common services and opaque, bound, expiring callbacks. Telegram owns no queue, ACL or replay state. |
| `TASK-260712-14rxuk` | Keep automated repository evidence distinct from manual B5-B7 and rollout evidence; do not promote this handoff into a gate result. |
| `TASK-260712-1kfnpu` | Review the exact accepted implementation/contracts, migration evidence and residual manual boundary before integrated Phase 2 acceptance. |
| `TASK-260712-2ys1ww` | Bind protected-media grants and authenticated data to the exact immutable target snapshot. Unsupported combinations fail or require explicit reviewed confirmation; plaintext fallback is forbidden. |
| `TASK-260712-3w1cst` | Add only ciphertext/public epoch/envelope metadata. Do not replace target, inbox, delete, retention, report or audit authority; predecessor rollback leaves new tables off and intact. |
| `TASK-260712-1rziyo` | Historical protected access requires an explicit bounded grant. Current membership, recovery, replacement device or inbox visibility must not inherit old content keys. |
| `TASK-260712-2i0w6x` | Metadata-only report stays canonical. A decrypted evidence copy requires a separate deliberate recipient action, consent, scoped storage and audit. |
| `TASK-260712-3u5cdn` | Execute real packaged Windows/macOS/Telegram B5-B7, accessibility, audible replay/no-autoplay, mixed-fleet and real fetch/revocation checks. |
| `TASK-260712-3qybi2` | Execute the production-shaped backup, additive migration, single-writer drain, exact predecessor rollback and roll-forward rehearsal. |

No consumer may derive authorization from current Air membership, presentation
action hints, a copied opaque handle, a cache object, report count or recovery
success. Any semantic change to selector binding, target snapshots, inbox
eligibility/TTL, cursor binding, replay lineage, content-policy versioning or
report side effects requires a versioned contract change before code.

## Evidence and manual boundary

Repository automation is indexed by the
[B5-B7 regression evidence](p2-targets-inbox-parity-regression-evidence.md).
The handoff validator additionally pins rollout order, preservation tables,
tested predecessor revisions, downstream task ownership, diagrams and the two
manual gates:

```sh
python3 scripts/validate_targets_inbox_contract.py
python3 scripts/validate_targets_inbox_parity_regressions.py
python3 scripts/validate_targets_inbox_rollout_handoff.py
python3 -m unittest scripts/acceptance/test_targets_inbox_rollout.py
python3 scripts/acceptance/run_automated.py --suite all --run-id <run-id>
```

The source anchors include transactional schema reconciliation, eligible inbox
backfill, exact predecessor execution, adversarial non-target/frozen-audience
coverage, HTTP pagination/replay redaction and crash-resumable moderation.
Passing them closes the repository engineering handoff only. It leaves
`TASK-260712-3u5cdn` and `TASK-260712-3qybi2` explicitly
`manual-required`.
