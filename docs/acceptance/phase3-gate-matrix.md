# Phase 3 gate matrix and evidence contract

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-3da0vz`

Status: contract frozen; no manual, independent-review, beta or promotion result executed

The normative contract is
[`acceptance/phase3/gate-matrix-v1.json`](../../acceptance/phase3/gate-matrix-v1.json).
This document explains its use; it does not turn any open gate into a pass.

## Claim boundary

Repository checks may prove deterministic protocol, lifecycle, boundedness and
synthetic-fixture properties. C1-C7 final results need signed applications and
the named real lab path. Independent review needs a reviewer other than the
implementers. The beta needs seven consecutive real 24-hour records. A missing
person, machine, network, credential, daily record or external action is a
blocker, never a substitute result.

No failed platform, route, direction, workflow or flag posture may be averaged
away. `live_ptt`, `e2ee_media`, `soundboard_cues` and `automation` have separate
promotion decisions. In particular, live PTT can pass while E2EE stays held,
and manual soundboard can pass while automation stays disabled.

## Provenance and fixtures

Every result binds one root commit, coordinator and signed-client hashes,
configuration/flag hash, schema, dependency and fixture hashes, environment,
network/clock information, and opaque operator/reviewer/consent record ids.
C1-C7, reviews, drills and beta must consume the same reviewed artifact set.
An affected binary, config, flag, schema, fixture, dependency, topology or
measurement-script change requires a root delta review and reruns affected
cells; a beta-affecting change resets the beta.

The frozen capture corpus is content-addressed by
`capture-quality-harness-lock-v1.json`. Live PTT uses the repository transport
model only as preflight. E2EE and automation preflights bind their checked-in
threat/lifecycle and engineering-evidence contracts. None is physical evidence.

## Environment and flag roster

The directed platform set is Windows→Windows, Windows→macOS, macOS→Windows and
macOS→macOS. Required physical rows are Windows 10 x64, Windows 11 x64,
Windows 11 ARM64 when shipped, and macOS 14+ ARM64. Capture covers speaker,
headphone and unknown routes for recorded clip, local self-test and live PTT.
C2 and beta require at least two real homes, three participants and two distinct
Internet connections. The actual hardware/network/participant roster is still
missing.

All 16 binary postures of `live_ptt`, `e2ee_media`, `soundboard_cues` and
`automation` are frozen as IDs `0000` through `1111`. Automation without
soundboard is an expected rejected configuration. Other valid postures keep
their claims separate; flag-off behavior is evidence, not an omitted cell.

## C1-C7 map

| Gate | Final owner/path | Frozen requirement |
| --- | --- | --- |
| `C1` | `TASK-260712-flaiie` / hold-release lab | 100 cycles per foreground app; no stuck/lost release or capture after lock |
| `C2` | `TASK-260712-flaiie` / two-home directed pairs | mouth-to-ear p50 ≤800 ms, p95 ≤1500 ms at 2% loss; intelligible; main program recovers |
| `C3` | `TASK-260712-2e80pr` then `TASK-260712-flaiie` | Windows/macOS speaker/headphone/unknown, clip/self-test/live; frozen acoustic rubric; no averaging |
| `C4` | `TASK-260712-yj668d` | removed member gets no new plaintext; new device gets no ungranted history |
| `C5` | `TASK-260712-yj668d` | coordinator storage/traffic cannot recover content; metadata matches the model |
| `C6` | `TASK-260712-yj668d` | voluntary recipient evidence copy only; no hidden server decryption |
| `C7` | `TASK-260712-1gyohk` | timezone/DST/no-catch-up, DND, ceiling, at-most-once, immediate revoke and no microphone |

C4-C6 and `e2ee_media` remain held by the deferred E2EE design,
implementation and external-review gate. The contract records their future lab
paths without implying implementation or acceptance.

## Section 21.4 and exit gates

`NF-jitter` and `NF-reconnect` cover bounded live buffers and generation-safe
reconnect. `NF-secure-storage` covers OS storage and log/crash redaction.
`NF-external-security-review`, `NF-root-review`, `NF-realtime-review`,
`NF-automation-review`, `NF-privacy-store-review` and
`NF-migration-recovery-review` bind findings and retests to the exact root.
`NF-disclosures` requires real public policy/Store records before rollout.
`NF-rollout-recovery` exercises every valid enable/hold/disable posture and
recovery boundary. `NF-beta` consumes seven consecutive valid days only after
the reviews and drills.

A stuck capture, runaway automation, defect-caused key loss, open critical/high
finding or missing daily record stops the cohort. Any prohibited incident or
material tested-path/config/fixture change preserves sanitized evidence,
disables the affected flag, requires review, and restarts at day one.

## Artifact layout and privacy

Campaign IDs are `p3-YYYYMMDDTHHMMSSZ-<12hex-root-head>` beneath the private
`.temp/acceptance/phase3` root. Each result lives under gate, flag posture,
pairing/role and run ID and carries result, manifest, commands, environment,
metrics and sanitization records. Manifests bind relative paths, byte lengths
and SHA-256 and become append-only after signing.

Raw audio, traffic payload, key material and participant data never enter git.
Private raw evidence defaults to 30 days; sanitized manifests default to 365
days. Exports use opaque roles and campaign-scoped hashed object IDs, never raw
actor/chat IDs, email, transcript, filename, token or local path.

## Approved inputs and explicit blockers

Approved defaults record Relux Works and Ivan Oparin as accountable owner, the
approved support/moderation mailboxes, USA hosting/data/backup posture, age 13+,
Armenian law/courts, English control language, and Ivan Oparin's Partner Center
submit/withdrawal and EN/RU review authority. The approved policy URLs are the
privacy, terms, content-guidelines and support routes under
`https://barycenter.live/legal/`. Moderation coverage is Monday-Friday
10:00-19:00 GMT+4 with two-business-day normal and 24-hour urgent targets.
Legal counsel review is explicitly not required; this is an approved decision,
not an open reviewer slot.

Still open: the physical Windows/macOS/audio roster; two-home and beta
participants; deferred E2EE work; independent crypto/realtime/automation/
privacy/migration reviewers; real public-policy/mail-delivery/Store
verification; and the Phase 3 observability export. Their exact blocked gates
are in the machine contract.

## Downstream execution rule

Validate the contract before any campaign:

```sh
python3 scripts/validate_phase3_gate_matrix.py
```

Copy `acceptance/phase3/result-template-v1.json` per cell and never edit the
frozen contract to fit a result. Validate a private campaign with:

```sh
python3 scripts/validate_phase3_campaign.py \
  --contract acceptance/phase3/gate-matrix-v1.json \
  --campaign .temp/acceptance/phase3/<campaign-id>
```

Only `TASK-260712-1actom` may add `--require-beta`, after all prerequisite
reviews and drills. A failed or blocked result stays in its exact cell. Later
promotion packets index signed sanitized manifests and cannot reinterpret an
engineering preflight as final evidence.
