# Phase 2 gate matrix and evidence contract

- Date: 2026-07-16
- Task: `TASK-260712-14rxuk`
- Contract: `p2-gate-matrix-evidence.v1`
- Machine-readable matrix:
  [`acceptance/phase2/gate-matrix-v1.json`](../../acceptance/phase2/gate-matrix-v1.json)
- Result template:
  [`acceptance/phase2/result-template-v1.json`](../../acceptance/phase2/result-template-v1.json)

This document freezes the shared pass/fail criteria, topology, samples, clocks,
provenance, artifact layout and privacy boundary that every later Phase 2
engineering review and manual acceptance task must consume. It does not run a
real application, create a production rollout or claim any B1-B7 result.

## Claim boundary and current blocker

Repository automation is preflight evidence only. It may prove deterministic
schema, wire, range, cache, lifecycle, ACL, rights and synthetic fanout
invariants. A final result requires the exact signed packaged application on
recorded real hardware; the seven-day result additionally requires seven
consecutive immutable 24-hour records in an approved real multi-home Air.

The accepted codec/player decision is currently `no-go`. No production
combination is selected, `stream_track_v1` is not advertised and the current
artifact can execute only dark rollout stages 1-4. Therefore `B1`, streamed
parts of `B6`, `20.5-track-start`, `20.5-start-skew`, `20.5-seek`,
`20.5-memory`, activation in `18-rollout` and `20.6-beta` are blocked before
manual execution. They reopen only after a reviewed replacement ADR, explicit
production encoder/decoder registries, an additive policy-schema revision and
exact signed Windows/macOS package hashes.

Developer-mode playback, a mock clock, synthetic fanout or a repository test
can never be relabelled as packaged, audible, hardware, production-shaped
rollback or beta evidence.

## Provenance, fixtures and clocks

Every campaign records the root commit; coordinator, MSIX and macOS app hashes;
package/signing identities; notarization receipt; exact configuration and
feature-flag hash; database schema/data-shape hash; fixture-lock hash; OS,
architecture, machine, audio device and network profile; and opaque operator
and consent record IDs. A binary, config, schema, fixture, topology or
measurement-script change invalidates the affected group. During beta it
restarts day one after root review.

The shared fixture recipe is `p2-codec-spike-rubric.v1`, exact SHA-256
`8701ddc6307e289817b020f07aa56b74203e472894b65ec84e4f61d5c9f6a4d2`,
generated only by FFmpeg 8.1.2 from its pinned release digest. It contains
one-hour and two-hour MP3, AAC/M4A, AAC/ADTS and Opus/Ogg classes plus hostile
inputs. Generation creates a content-addressed lock; all later samples bind the
same lock hash. A replacement ADR must name the actual production subset. With
the current no-go there is no permitted final B1 fixture selection.

Latency uses process monotonic clocks. Scheduled skew uses the coordinator's
monotonic schedule and each node's monotonic first-audio event. UTC wall time is
metadata only. Cross-node sampling records its sync source and pre/post offset;
absolute offset above 10 ms invalidates the sample. RSS, disk, network and
queue series use a 1 Hz clock. Failures stay in the denominator.

## Platform, topology and sample roster

The required pairings are Windows↔Windows, Windows↔macOS and macOS↔macOS with
two distinct installations in each pairing. Windows coverage includes a
physical Windows 10 x64 build 19044 row and a supported recorded Windows 11 x64
row; Windows 11 ARM64 is required if that architecture is shipped. macOS uses
physical arm64 hardware with macOS 14 or later and a signed notarized app.

The real Air minimum is three Barycenters and five Pulsars. The scale shape is
eight Barycenters and twenty Pulsars. Mixed fleet contains at least one Phase 1
node and one exact Phase 2 build in the same accepted snapshot. Real Telegram
uses a private chat and opaque callbacks; raw chat/user IDs never enter the
export.

Timed groups run three warmups and thirty measured samples, using nearest-rank
p95 grouped by build, pairing, node, fixture and network profile. There is one
complete one-hour playback and one two-hour duration run per pairing. B2-B4
repeat thirty times. B5-B7 run thirty adversarial/policy/revocation cases per
applicable surface. Synthetic scale runs thirty iterations of twenty commands
at 8×20 and permits zero duplicate or lost commands.

## B1-B7 evidence map

| Gate | Automated preflight | Final artifact and owner |
| --- | --- | --- |
| `B1` | Full pinned repository acceptance manifest | Story-level real-app precondition `TASK-260712-1fpb9q`, then pairing result from manual `TASK-260712-2bdi4a`; blocked by codec no-go |
| `B2` | Air regression artifact for exact current union/fanout | 3-Barycenter/5-Pulsar result from manual `TASK-260712-21kz3b` |
| `B3` | Air join/catch-up/no-old-overlay regressions | Living-Air result from manual `TASK-260712-21kz3b` |
| `B4` | Air leave/isolated-cancel regressions | Leave-during-track result from manual `TASK-260712-21kz3b` |
| `B5` | Targets/inbox adversarial ACL contract | Explicit-target result from manual `TASK-260712-3u5cdn` |
| `B6` | Capability/mixed-version and no-fallback contracts | Stream real-app precondition `TASK-260712-1fpb9q`, then mixed-fleet result from manual `TASK-260712-3u5cdn`; stream portion blocked by codec no-go |
| `B7` | Consent, report-local and canonical revocation contract | Rights/abuse result from manual `TASK-260712-3u5cdn` |

Every row names its exact command and artifact pattern in the machine-readable
matrix. A preflight pass does not alter the manual row's status.

## Sections 17, 18, 20.5 and 20.6

`20.5-track-start` is nearest-rank p95 ≤5,000 ms, `20.5-start-skew` is
p95 ≤100 ms, `20.5-seek` is p95 ≤3,000 ms, and `20.5-memory` is maximum
process-tree RSS ≤200 MiB. Timed gates use three warmups and thirty measured
samples. RSS is sampled once a second and must remain duration-independent
under the codec rubric's growth/slope gates.

`20.5-accounting` permits zero reconciliation mismatch or unexplained
crash-release growth and remains dependent on `TASK-260712-qi81vf` for the
canonical sanitized export. `20.5-migration` permits zero dual-runtime command,
lost Phase 2 row or Phase 1 regression and remains manual in
`TASK-260712-3qybi2`. `20.5-scale` is thirty 20-command iterations at 8×20 with
zero loss/duplication; its existing synthetic result is preflight only.

`17-observability` must consume canonical counters and is engineering-pending
in `TASK-260712-qi81vf`. `18-rollout` follows additive schema → dark
coordinator → dark clients → replacement ADR/runtime gate → internal orbit →
bounded expansion → public promotion, with manual production-shaped rehearsal
in `TASK-260712-3qybi2`. `20.6-beta` starts only after reviews and B1-B7: seven
consecutive 24-hour windows, seven immutable daily records and zero critical
incidents. A critical incident or unapproved build/config change stops the
cohort, invokes approved drain/rollback and restarts day one after a fix and
root review.

## Artifact layout and privacy

Campaign IDs are `p2-YYYYMMDDTHHMMSSZ-<12hex-root-head>`. Raw working data is
under `.temp/acceptance/phase2/<campaign-id>` and each result uses
`<gate>/<pairing-or-topology>/<run>/<artifact-kind>.<ext>`. Every run has a
result, artifact manifest, command list, environment record, metrics CSV and
sanitization report. Manifests bind relative path, byte length and SHA-256 and
become append-only after signing. The final packet references sanitized signed
manifests and content-addressed artifact URIs, not raw files.

Ivan Oparin owns participant-consent approval. Audio bytes, original filename,
caption, transcript, bearer tokens, raw actor/chat IDs, emails and local paths
are forbidden in ordinary artifacts. Campaign-scoped hashed media or
transmission IDs and opaque installation roles are allowed. Sanitization is a
required artifact, not an informal note.

## Explicit blockers and downstream handoff

- Codec/player no-go blocks final `B1`, streamed `B6`, four timing/memory gates,
  activation in `18-rollout` and `20.6-beta`.
- Real hardware/lab, participant consent and production credentials/flag
  authority are manual inputs owned by Ivan Oparin under
  `EPIC-260714-th54l3`.
- `TASK-260712-qi81vf` owns the missing canonical `17-observability` and
  `20.5-accounting` export.
- `TASK-260712-1fpb9q`, `TASK-260712-2bdi4a`, `TASK-260712-21kz3b`, `TASK-260712-3u5cdn`,
  `TASK-260712-3qybi2` and `TASK-260712-2pnc5a` execute final manual evidence.
- `TASK-260712-1kfnpu` and `TASK-260712-3a0cf9` must preserve this claim
  boundary and may not convert an absent result into a pass.

Validate the frozen contract with:

```sh
python3 scripts/validate_phase2_gate_matrix.py
python3 -m unittest scripts/acceptance/test_phase2_gate_matrix.py
```

Validate an eventual campaign with:

```sh
python3 scripts/validate_phase2_campaign.py \
  --contract acceptance/phase2/gate-matrix-v1.json \
  --campaign .temp/acceptance/phase2/<campaign-id>
```

Passing the contract validator proves consistency only. It leaves every manual
result, production enablement and beta record unexecuted.
