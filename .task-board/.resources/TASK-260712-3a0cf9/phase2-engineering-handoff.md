# Phase 2 engineering handoff

- Task: `TASK-260712-3a0cf9`
- Date: 2026-07-16
- Handoff baseline: `cfb6fa3801742e1150ca22d95452093efe2c037d`
- Root-reviewed source: `5f2f7e97a343b4bca84fe56ee57dd02458265f31`
- Root-reviewed tree: `4a03b4d3a3db062ed210e6696869366a9b6cf775`
- Decision: Phase 2 repository engineering complete; production promotion
  blocked; reversible P3 development may start
- Machine-readable index:
  [`engineering-handoff-v1.json`](../../acceptance/phase2/engineering-handoff-v1.json)

## What this packet accepts

This packet is the single entry point for reproducing the Phase 2 engineering
state without chat history. It accepts the exact root-reviewed source tree as a
reversible engineering baseline. The complete root inventory covers 624
no-rename paths, 50 Phase 2 tasks and every B1-B7 mapping with zero unmapped
paths. The clean local workflow passed all 12 stages and 89 contract tests;
hosted root run `29509397804` passed coordinator, Windows, macOS and signed
packaged-probe jobs.

There is no open critical or high defect in the active, disabled-production
repository baseline. Thirteen High findings remain as explicit production
gates: codec/legal/supply decisions, a selected-codec bounded integrity design,
real hardware/campaign evidence and independent approvals. They are neither
closed nor downgraded here.

This packet does **not** accept a production build, Windows package, macOS
package, runtime configuration, database snapshot or fixture lock. Every such
hash is `null`. It does not claim B1-B7, rollout, audible behavior, real
Telegram behavior, seven beta days or Store/public promotion.

## Reproduce the repository state

From a clean checkout containing the handoff baseline or a descendant:

```sh
python3 scripts/acceptance/validate_phase2_engineering_handoff.py
python3 scripts/acceptance/validate_p2_root_review.py
python3 scripts/validate_phase2_gate_matrix.py
python3 scripts/acceptance/validate_phase2_observability.py
python3 scripts/validate_streamed_track_rollout_handoff.py
python3 scripts/validate_targets_inbox_parity_regressions.py
python3 scripts/validate_targets_inbox_rollout_handoff.py
python3 scripts/acceptance/run_air_regression.py \
  --output .temp/acceptance/phase2-handoff-air-regression.json
python3 scripts/acceptance/run_automated.py \
  --suite all --require-clean --run-id <unique-run-id>
```

The handoff validator hashes 27 contracts, reviews, runbooks, source
authorities and validation tools from the immutable handoff baseline. The
root manifest independently regenerates its exact candidate inventory and
reads task cards from that candidate's Git tree, so later board status changes
cannot rewrite review provenance.

The Air command above passed in this handoff review with two command groups,
zero failures and the expected synthetic 8-Barycenter/20-Pulsar shape: one
runtime, 20 unique targets, 20 commands, zero duplicates and zero legacy
groups. Its result is repository preflight only.

## Evidence index

| Area | Canonical artifact | Repository result | Final boundary |
| --- | --- | --- | --- |
| gate/lab contract | `acceptance/phase2/gate-matrix-v1.json` | 17 gates frozen; provenance, clocks, samples, privacy and beta reset rules validated | no final campaign executed |
| codec and supply | `acceptance/codec-spike/player-handoff-v1.json`, `independent-supply-review-v1.json` | deterministic no-go retained; production registries empty | six High supply/legal/package gates open |
| root review | `acceptance/phase2/root-integration-review-v1.json`, root report and manifest | exact source accepted; 624 paths, zero unmapped | production build and package hashes null |
| streamed tracks | `acceptance/streamed-track-rollout-handoff-v1.json` | schema/range/cache/player/accounting and rollback contracts pass with production disabled | B1/B6 and performance matrix manual; selected codec absent |
| Air | `docs/analysis/p2-air-lifecycle-regression-rehearsal.md` plus synthetic runner | lifecycle, migration, cutover, rollback and 8x20 fanout preflight pass | B2-B4 real topology and independent approval pending |
| targets/inbox/rights | parity and rollout JSON contracts | B5-B7 ACL, no-broadcast, consent, cursor and revocation contracts pass | real Telegram/mixed fleet/rights campaign pending |
| observability/quotas | `observability-contract-v1.json` and runbook | authenticated canonical view, flag-aware health and fixed dimensions pass | real client timings, capacity and seven-day calibration pending |
| rollback | streamed, target and Air handoffs plus exact-predecessor tests | additive rows preserved; flag/caller withdrawal and drain order frozen | production-shaped rehearsal pending |

The full path/digest inventory is in `sourceAnchors` in the machine-readable
packet. The root-review tracking update initially failed hosted run
`29509872003` because the manifest generator read mutable task status from the
worktree. Commit `c62ac201d4d07ab8f61186369a0fa80c6d73ba93` corrected it to
read the reviewed candidate tree. Hosted run `29510066254` then passed all four
jobs. The failure is retained as review provenance rather than hidden.

## B1-B7, non-functional, rollout and beta status

| Gate | Repository evidence | Required final task(s) | Status |
| --- | --- | --- | --- |
| B1 | full Go/Swift contracts, range/cache/player state and disabled registry | `TASK-260712-1fpb9q`, `TASK-260712-2bdi4a` | manual, codec-blocked |
| B2 | exact active-Air union and synthetic fanout | `TASK-260712-21kz3b` | manual |
| B3 | saved/active lifecycle, join readiness and restart state | `TASK-260712-21kz3b` | manual |
| B4 | leaver-only cancel/fade and personal restore | `TASK-260712-21kz3b` | manual |
| B5 | exact opaque targets, no broadcast and inbox pagination | `TASK-260712-3u5cdn` | manual |
| B6 | mixed-version policy, no fallback and visible unsupported receipts | `TASK-260712-1fpb9q`, `TASK-260712-2bdi4a`, `TASK-260712-3u5cdn` | manual, codec-blocked |
| B7 | reporter-local protection plus canonical global revocation | `TASK-260712-3u5cdn` | manual |
| 20.5 | thresholds, clocks, samples, synthetic load, accounting view and migration contract | the six Phase 2 manual tasks below | manual |
| section 18 rollout | dark deploy, caller withdrawal, drain, predecessor and roll-forward order | `TASK-260712-3qybi2` | manual, maximum stage 4 |
| 20.6 beta | seven immutable 24-hour records and incident reset rule | `TASK-260712-2pnc5a` | not started |

Repository tests are never represented as audible, packaged-hardware or beta
evidence. A mock clock is not real latency, synthetic fanout is not audible
playback, and a developer build is not a signed installed application.

## Feature authority and rollback ownership

### Streamed tracks

`stream_variant_policy` permits only
`production_selection_enabled=0` with an empty profile. The production encoder
and decoder registries are empty and Windows/macOS advertise no
`stream_track_v1`. There is deliberately no `streamed_tracks` runtime switch:
adding one before a replacement ADR would bypass the guard instead of safely
controlling it.

A future replacement must select one exact Windows+macOS combination, close
license/signing/Store gates, add explicit registries and revise the policy
schema additively. Only then may a coordinator-owned default-off per-orbit
allowlist exist. Rollback ownership belongs to the operator: withdraw upload,
queue, replace, replay and Telegram track callers; drain/cancel generations;
stop the sole SQLite writer; preserve additive rows; deploy the retained tested
predecessor; then reconcile before roll-forward.

### Air rooms

Air enablement is the persisted `air_authority.mode` and generation, not an
environment variable. `links_authoritative` and `airs_shadow` are dark states;
only `airs_authoritative` enables Air delivery. `rollback_hold` prevents an
Air-unaware predecessor from resurrecting legacy delivery. Link and Air
runtimes may never deliver concurrently. Operators own cutover/hold and must
preserve all Phase 2 rows.

### Explicit targets, inbox and rights

There is no invented coarse target flag. Authority is authenticated caller
exposure plus canonical target, inbox, consent and moderation stores. Rollback
withdraws create and replay callers before predecessor deployment while
history, receipts, delete, report, block and audited disable remain available.
Personal N-recipient work never broadcasts as fallback.

## Quota model and observability

The values below are binary-byte engineering defaults, not beta-calibrated
production decisions:

| Limit | Actor | Orbit |
| --- | ---: | ---: |
| upload starts / 24 h | 100 | 500 |
| input bytes / 24 h | 5 GiB | 25 GiB |
| canonical bytes | 10 GiB | 50 GiB |
| processing temp reservation | 2 GiB | 8 GiB |
| concurrent jobs | 2 | 8 |
| retained track bytes | 20 GiB | 100 GiB |
| egress / 24 h | 100 GiB | 500 GiB |

Only a revision-conditional audited moderation-operator decision changes a
policy. `TASK-260712-2pnc5a` owns real seven-day calibration. The authenticated
Phase 2 view consumes canonical accounting, playback, target/inbox and Air
state. It adds no parallel ledger. Client buffer, seek-to-audio and audible
timings stay `client_evidence_required` until a real campaign supplies them.

## Ordered rollout ceiling

The current artifacts document, but do not claim execution of, stages 1-4:

1. freeze hashes, backup and retain the tested predecessor;
2. deploy additive coordinator schema dark;
3. reconcile and observe dark with Phase 1 still green;
4. deploy clients/adapters dark without advertising streamed-track capability.

Stage 5 requires the replacement codec/player ADR. Stage 6 requires one
internal orbit and real B1. Stage 7 requires telemetry/incident review. Stage
8 requires B1-B7, production-shaped rollback, seven-day beta and owner
approval. This handoff grants permission for none of stages 5-8.

## Pending manual and external work

All 19 manual-real-app tasks remain pending in `EPIC-260714-th54l3`. The six
that close Phase 2 are:

- `TASK-260712-1fpb9q` — streamed-track regression evidence;
- `TASK-260712-21kz3b` — B2-B4 Air and scale acceptance;
- `TASK-260712-2bdi4a` — B1 track platform matrix;
- `TASK-260712-3qybi2` — rollout, migration and rollback;
- `TASK-260712-3u5cdn` — B5-B7 rights and mixed fleet;
- `TASK-260712-2pnc5a` — seven-day beta and quota calibration.

Implementation-independent and owner decisions remain in
`EPIC-260714-zmnd4n`, owned by Ivan Oparin:

- `TASK-260716-tlxe3s` — codec candidate, legal and supply decision;
- `TASK-260716-3voo6j` — streamed performance approval;
- `TASK-260716-19g4gd` — Air migration approval;
- `TASK-260716-2l5j1a` — target/range/rights security approval.

## P3 entry decision

This packet opens only reversible P3 development. The next manual row,
`TASK-260712-9wivva`, remains deferred in the manual epic. Strict engineering
execution therefore continues with `TASK-260712-lo7a68` —
`live-codec-transport-spike`. P3 work must not enable Phase 2 production,
reinterpret Phase 2 evidence as passed, or mutate any of the pending owner and
manual gates.
