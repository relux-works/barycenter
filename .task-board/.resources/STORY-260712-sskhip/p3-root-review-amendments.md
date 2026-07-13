# Phase 3 root-review amendments

Date: 2026-07-12  
Epic: `EPIC-260712-3agrc1`  
Authority: this document supersedes the original Phase 3 agent decomposition
notes where they conflict. The product source of truth remains
`docs/spec-self-contained-audio.md` version 0.2.

## Non-delegable review rule

No implementation-agent result is accepted from status, checklist, prose, or
self-reported tests alone. Before integrated acceptance, the root agent must:

1. read every implementation, protocol, migration, UI, packaging, test,
   dependency and operations diff line by line;
2. map every hunk to the source specification and task acceptance criteria;
3. inspect authorization, concurrency, bounded resources, rollback, privacy,
   realtime callbacks, cryptographic boundaries and mixed-version behavior;
4. rerun all locally available relevant and regression checks;
5. freeze the reviewed commit, build, fixture and dependency hashes;
6. reject unsupported agent claims and record exact hardware or external-only
   evidence still required.

Crypto, realtime audio, automation, privacy/Store and migration/recovery also
require independent reviewers. Any relevant change after a review invalidates
that review and requires a delta review. A final root audit runs after the beta
and promotion packet.

## Live PTT corrections

The original platform tasks were too broad and had no codec/transport gate.
The reviewed execution shape is:

1. Phase 2 promotion packet `TASK-260712-3a0cf9`.
2. In parallel:
   - signed hold-input/lost-release spike `TASK-260712-9wivva`;
   - live codec/transport spike `TASK-260712-lo7a68`.
3. Generation-safe bounded wire contract `TASK-260712-3qviqc`.
4. In parallel:
   - ephemeral coordinator relay `TASK-260712-3vzbbl`;
   - Windows sender `TASK-260712-ezdhpf`;
   - Windows jitter receiver `TASK-260712-1ckdr7`;
   - macOS sender `TASK-260712-26mnp1`;
   - macOS jitter receiver `TASK-260712-19w1qn`.
5. Platform integration shells `TASK-260712-2jbo5i` and
   `TASK-260712-2kj9kj`.
6. C1-C2 evidence `TASK-260712-1rzqh9`.

The codec task must pin an exact low-latency profile and transport, test 2%
loss, jitter, head-of-line behavior and slow recipients, and issue a no-go if
C2 is not credible. Live audio is never persisted. A remote command cannot
open the microphone. Lost release, lock/sleep, revoke, disconnect, backpressure
and quit must all terminate capture. Sender, decoder, network and waits remain
off realtime callbacks; all queues and jitter buffers are hard-bounded.

## Capture-quality corrections

AEC, noise suppression and AGC are not a live-only subsystem. One reusable DSP
path serves recorded clips, the local five-second record-then-play test and live
PTT. Input AGC ceiling and receiver output-volume ceiling are distinct.

Reviewed order:

1. Shared DSP graph/C3 contract `TASK-260712-1gmsvh`.
2. In parallel:
   - Windows signed voice-processing probe `TASK-260712-265o0f`;
   - macOS voice-processing probe `TASK-260712-2gaswa`;
   - shared capability/diagnostics schema `TASK-260712-1pw1l1`.
3. Deterministic DSP fixture harness `TASK-260712-39czd2`.
4. Platform processors `TASK-260712-wcdz08` and `TASK-260712-2egweh`.
5. Platform UIs `TASK-260712-39zh8g` and `TASK-260712-1getbv`.
6. Integrated clip/PTT regressions `TASK-260712-1023d7`.
7. Real C3 acoustic matrix `TASK-260712-2e80pr`.

The C3 rubric includes far-end-only, near-end-only, double-talk, echo-path and
route changes, clock drift, clipping and too-quiet cases. AEC uses a named
render reference and accepted speaker mode cannot silently continue when that
reference is unavailable. Objective measurements and reproducible blinded
listening are both required.

## E2EE corrections

The original plan incorrectly assigned key rotation to the coordinator and
combined crypto, media, UI and playback into two platform tasks. The coordinator
may order signed client-produced membership state and route opaque data, but it
must never create, unwrap, escrow or log group/content secrets.

Reviewed order:

1. Threat model and claims `TASK-260712-2e2ymn`.
2. In parallel:
   - audited group-crypto/library spike `TASK-260712-3er89x`;
   - protected container/local-preparation spike `TASK-260712-16xmy2`.
3. Protocol/key lifecycle/downgrade contract `TASK-260712-2ys1ww`.
4. Independent cryptographic design review `TASK-260712-aniuyy`.
5. Public-state/ciphertext schema `TASK-260712-3w1cst`.
6. Client-driven Air epoch coordination `TASK-260712-20j5tm`.
7. In parallel:
   - opaque media/live router `TASK-260712-1yz5ca`;
   - Windows key state `TASK-260712-25dzp4`;
   - macOS key state `TASK-260712-1x9ruo`.
8. Platform send, playback and encrypted-live tasks plus recovery and report
   services:
   - `TASK-260712-28zhpl`, `TASK-260712-2kcduo`;
   - `TASK-260712-1u57qz`, `TASK-260712-tcwn44`;
   - `TASK-260712-39vjzd`, `TASK-260712-3980vy`;
   - `TASK-260712-1rziyo`, `TASK-260712-2i0w6x`.
9. Honest platform UX `TASK-260712-2q4jbu` and
   `TASK-260712-2nppt6`.
10. C4-C6/code-review packet `TASK-260712-1bcpda`.

The design uses only audited maintained primitives/libraries selected by the
spike. It must define device identity, coordinator trust, group commits,
forward-secrecy claims, nonce/key separation, replay/fork handling and canonical
cross-platform vectors. Clips, tracks and saved cues use seekable chunked AEAD;
live PTT uses a fresh session key and authenticated frames before jitter decode.
There is no silent plaintext downgrade.

Revoke/delete cannot erase keys or plaintext already obtained by another
device. Recovery requires a surviving authorized device or an explicitly
threat-modeled user-held recovery capability; otherwise protected history can
be irrecoverable. Telegram uploads are not claimed as E2EE. A voluntary report
evidence copy visibly leaves the E2EE boundary, is purpose-limited, access-
controlled, audited and short-lived.

## Soundboard and automation corrections

A saved cue must not disappear under ordinary seven-day clip cleanup. It is an
owner-scoped, quota-accounted durable reference to canonical ready media or a
hash-pinned builtin asset, with shared ACL, report, delete and disable behavior.

Reviewed tasks include:

- threat-model/contract `TASK-260712-3sj8ox`;
- saved-cue lifecycle `TASK-260712-hb5xz2`;
- at-most-once schema `TASK-260712-3sv87k`;
- control APIs `TASK-260712-1kk8bd`;
- runtime/kill switches `TASK-260712-1eva0y`;
- history/audit `TASK-260712-11e4e3`;
- macOS and Windows soundboards `TASK-260712-288j4a`,
  `TASK-260712-1yw7fo`;
- macOS and Windows administration `TASK-260712-1oodka`,
  `TASK-260712-89fzlc`;
- Telegram quick-control parity `TASK-260712-uht9e2`;
- C7 evidence `TASK-260712-2f0gpu`.

Schedules use an IANA timezone and frozen DST/no-catch-up rules. Atomic claims
prevent duplicates after restart, concurrent ticks or clock jumps. Bearer
secrets are shown once and stored hashed. The coordinator rechecks DND, block,
Air, target, media and revoke state but does not enforce the recipient local
output ceiling; the recipient mixer remains authoritative. Manual soundboard
and automation have separate flags, and automation can never open a microphone.

## Final acceptance and release gates

`STORY-260712-2ft5wd` is intentionally not a self-review loop. Its order is:

1. Gate matrix `TASK-260712-3da0vz`.
2. Observability/evidence surfaces `TASK-260712-2uo81g`.
3. Non-delegable root line review `TASK-260712-3g0axs`.
4. Independent reviews in parallel:
   - external crypto implementation `TASK-260712-1ulshp`;
   - realtime audio `TASK-260712-3j4a06`;
   - automation `TASK-260712-1x5jfo`;
   - privacy/Store `TASK-260712-7ng1vs`.
5. Final C1-C3, C4-C6 and C7 matrices `TASK-260712-flaiie`,
   `TASK-260712-yj668d`, `TASK-260712-1gyohk`.
6. Rollout/rollback/recovery drills `TASK-260712-30xwu2`.
7. Independent migration/recovery review `TASK-260712-6mz9xg`.
8. Seven consecutive real beta days `TASK-260712-1actom`.
9. Promotion/hold packet `TASK-260712-3b7bp4`.
10. Non-delegable root final release audit `TASK-260712-2b5685`.

The beta resets after a prohibited incident, open critical/high finding,
missing daily evidence, or material tested-path code/configuration change.
Promotion is decided independently for `live_ptt`, `e2ee_media`,
`soundboard_cues` and `automation`; a held feature stays disabled and cannot
borrow claims from another passing feature.

## Inputs that cannot be invented

- real supported Windows and macOS hardware and two home networks;
- real multi-home beta participants;
- qualified independent cryptographic reviewer;
- independent realtime, automation, privacy/Store and operations reviewers;
- real legal entity, privacy/support contacts, moderation mailbox and policy
  hosting;
- actual Partner Center submission authority and current Store metadata.

Missing inputs block only the affected evidence or release gate. They are never
replaced by fabricated credentials, screenshots, identities, reviewer sign-off
or simulated seven-day evidence.
