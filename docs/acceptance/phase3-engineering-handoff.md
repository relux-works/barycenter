# Phase 3 engineering handoff

- Task: `TASK-260712-3b7bp4`
- Date: 17 July 2026
- Handoff baseline: `5e2daffa784f42dc2e736714f537cdf99d3e873b`
- Root-reviewed non-E2EE source: `d94f51644a3acf37601b4a869b4247380372f9ec`
- Root-reviewed tree: `4e4cca878db806650eda6f1e1642051b87a18b93`
- Decision: handoff ready for the root engineering completion audit; every
  capability remains held from production
- Machine index:
  [`engineering-handoff-v1.json`](../../acceptance/phase3/engineering-handoff-v1.json)
- Disclosure delta:
  [`phase3-disclosure-delta.md`](../compliance/phase3-disclosure-delta.md)

## Accepted boundary

The root line review accepted the exact non-E2EE repository source after two
High findings were fixed and re-reviewed. Its deterministic inventory covers
420 no-rename paths, 1,700 aggregate hunk headers and zero unmapped paths.
Subsequent realtime, automation, privacy/Store and migration/recovery packets
found no new open Critical or High technical defect. Their inline reviews are
engineering pre-reviews, not implementation-independent approvals.

This handoff accepts an indexed, reproducible repository state only. It does
not accept a production build, Windows package, macOS package, runtime config,
database snapshot, final fixture lock, physical result, independent review,
public policy, Partner Center record, rollout drill, beta day or release. All
production hashes are intentionally `null`.

## Reproduce the repository evidence

Run from a clean checkout containing the baseline or a descendant:

```sh
python3 scripts/acceptance/validate_phase3_engineering_handoff.py
python3 scripts/acceptance/validate_p3_root_review.py
python3 scripts/validate_phase3_gate_matrix.py
python3 scripts/acceptance/validate_phase3_observability.py
python3 scripts/acceptance/validate_p3_realtime_pre_review.py
python3 scripts/acceptance/validate_p3_automation_pre_review.py
python3 scripts/acceptance/validate_p3_privacy_store_pre_review.py
python3 scripts/acceptance/validate_p3_migration_recovery_pre_review.py
python3 scripts/acceptance/run_automated.py \
  --suite all --require-clean --run-id <unique-run-id>
```

The handoff validator reads 35 source authorities from the immutable baseline,
checks every digest, verifies the root tree and review decisions, and rejects
promotion, manual, independent, E2EE, publication, Store and production-artifact
claims. Later changes to an anchored source require a dated delta review.

## Review provenance

| Domain | Exact packet | Merge and hosted run | Result and remaining owner |
| --- | --- | --- | --- |
| root non-E2EE | `7388459356ec3a6ed976cdc779fec939adfa8d7b` | PR #256, `0d6f85d…`, run `29582027620` | 420 paths, zero unmapped, two High fixed/re-reviewed; production withheld |
| realtime | `68afff5295ad395985d04cb18efc2872544e439c` | PR #258, `2ad49cd…`, run `29583827330` | clean 7/7; independent `TASK-260717-3dbi2v`; manual `TASK-260712-flaiie` |
| automation | `a1dae4856f4bafa0c7679fddc19e3661691a4812` | PR #260, `e41f171…`, run `29585744116` | clean 7/7; independent `TASK-260717-1pyg62`; manual `TASK-260712-1gyohk` |
| privacy/Store | `5784985d02feb0471cc7cb389c7d3141dfad12b7` | PR #262, `bb2adae…`, run `29587384257` | clean 7/7; independent/publication/Store `TASK-260717-35bll1` |
| migration/recovery | `e68b59e1adb7d1ba586e8808800dfd249dab80eb` | PR #264, `8f5c15a…`, run `29589199967` | clean 7/7; independent `TASK-260717-1sgb5r`; manual `TASK-260712-30xwu2` |

The broad timed-out realtime and automation attempts remain recorded in their
source packets as non-counted. Scoped frozen groups passed; the handoff does
not hide the attempts or reinterpret them as failures or passes.

## Capability recommendation

| Capability | Repository conclusion | Final recommendation |
| --- | --- | --- |
| `live_ptt` | Protocol, coordinator, Windows/macOS capture and jitter paths, integration, observability and review preflight exist. `DUET_LIVE_PTT` defaults off and production clients do not advertise the capability. | **hold** pending live regression, C1-C3, independent realtime review, signed rollback/recovery, beta and root audit |
| `e2ee_media` | Threat model, two no-go spikes and an audit-only protocol contract exist. There is no selected crypto implementation, protected-media path, key lifecycle, advertised capability or recovery evidence. | **deferred unavailable** to `EPIC-260716-3qsztl`; no E2EE claim is allowed |
| `soundboard_cues` | Saved-cue lifecycle, desktop/Telegram surfaces and deterministic safety evidence exist. Audible, signed-app, accessibility and physical controls are not run. | **hold** pending C7/manual evidence, independent automation/privacy reviews, rollout, beta and root audit |
| `automation` | Schedules, principals, idempotency, policy rechecks, history, revoke and emergency disable are repository-proved. | **hold** pending C7, independent automation/privacy reviews, rollout, beta and root audit |

Live PTT and current stored/cue audio are coordinator-readable and are **not
end-to-end encrypted**. HTTPS/WSS, OS credential storage, the container spike
and protocol vectors do not change that statement.

## C1-C7 and section 21.4 index

The exact artifact patterns and all closure tasks are in `gateIndex` in the
machine packet. This summary preserves the decision boundary:

| Gate group | Repository evidence | Final state |
| --- | --- | --- |
| C1-C2 | deterministic lifecycle/transport, root and realtime pre-review | physical directed-platform/audio matrix not run |
| C3 | 18 independent synthetic cells and 252 fixture runs | native AEC/NS, acoustic, listening, route and signed-app evidence not run |
| C4-C6 | threat and audit-only protocol contracts | deferred E2EE implementation/reviews and manual matrix not run |
| C7 | deterministic automation safety handoff and pre-review | signed-app, physical clock, audible/accessibility and independent evidence not run |
| jitter/reconnect | bounded receiver/generation tests and sanitized observability | client mouth-to-ear, jitter and reconnect campaign not run |
| secure storage/external security | contract requirements only | blocked by deferred E2EE and independent implementation review |
| root review | exact non-E2EE source accepted | production and whole-epic acceptance withheld |
| independent domain reviews | four technical pre-review packets | all four implementation-independent owner tasks remain backlog |
| disclosures | EN/RU draft and exact Live PTT/E2EE boundary | publication, mail routing, screenshots, WACK, IARC and Partner Center not run |
| rollout/recovery | additive migration and kill-path contracts | signed mixed-fleet drills not run |
| beta | seven-day and reset rubric frozen | zero real beta days accepted |

No gate is averaged across capabilities, platforms, routes or flag postures.
`not-run`, missing environment and missing reviewer stay distinct from a failed
measurement and from a pass.

## Disclosure state

The current Phase 1 EN/RU privacy and listing sources remain controlling and
truthfully say coordinator-readable audio is not E2EE. The Phase 3 Store draft
is conditional and has `eligibleForSubmission=false`. It adds exact non-E2EE
Live PTT wording, keeps E2EE unavailable, and does not reduce the conservative
IARC private-user-audio/UGC answers.

Ivan Oparin owns EN/RU review, final submission and emergency withdrawal.
`TASK-260714-200ib8` owns mailbox routing, `TASK-260715-24ube9` owns the Phase 1
Partner Center prerequisites, and `TASK-260717-35bll1` owns independent Phase 3
privacy/Store acceptance and exact live publication/portal evidence.

## Rollback boundary

- Live PTT: unset `DUET_LIVE_PTT`, restart the coordinator, verify capability
  withdrawal and drain zero active capture/session state.
- Soundboard: withdraw trigger callers, retain delete/report/history controls,
  drain canonical transmissions and preserve additive cue/media rows.
- Automation: revision-condition disable or emergency-disable the orbit, revoke
  principals, disable schedules, reconcile cancellation, preserve audit/history/
  idempotency rows, then deploy a retained predecessor only after work is
  terminal.
- E2EE: there is no production enable path; keep the capability absent and do
  not reinterpret audit-only recovery contracts as an implemented recovery.

These are documented commands and order, not proof that a signed mixed fleet
executed them. `TASK-260712-30xwu2` owns that manual evidence.

## Complete deferred inventory

All 19 real-app/hardware tasks remain backlog in `EPIC-260714-th54l3`:

- Phase 1: `TASK-260712-1vtwkl`, `TASK-260712-2hodti`,
  `TASK-260712-e5mfqj`;
- Phase 2: `TASK-260712-1fpb9q`, `TASK-260712-21kz3b`,
  `TASK-260712-2bdi4a`, `TASK-260712-2pnc5a`, `TASK-260712-3qybi2`,
  `TASK-260712-3u5cdn`;
- Phase 3 platform: `TASK-260712-9wivva`, `TASK-260712-1rzqh9`,
  `TASK-260712-265o0f`, `TASK-260712-2gaswa`, `TASK-260712-2e80pr`;
- Phase 3 release: `TASK-260712-flaiie`, `TASK-260712-yj668d`,
  `TASK-260712-1gyohk`, `TASK-260712-30xwu2`, `TASK-260712-1actom`.

All 18 remaining E2EE design, implementation, client, review and evidence tasks
remain backlog in `EPIC-260716-3qsztl`; the full ordered ID list is frozen in
`deferredE2EE.tasks` in the machine packet.

## Next strict task

This packet authorizes only `TASK-260712-2b5685` — the root engineering
completion audit. It does not authorize capability activation, beta, Store
submission or release. The whole original epic remains open until every
applicable external/manual/deferred gate closes and the final audit is accepted.
