# Implementation plan: self-contained Pulsar Audio

Date: 2026-07-12  
Epic: `EPIC-260712-3agrc1`  
Status: planning complete; implementation requires explicit user approval.

## Objective and authority

Make Pulsar a self-contained private audio channel that works without mandatory
Spotify or Telegram, while preserving both as optional integrations. Deliver in
three independently gated phases: Store-ready clips, Air plus long audio, and
near-live PTT plus quality, E2EE and safe automation.

Authoritative inputs:

- `.spec/self-contained-audio.md` — execution entry point;
- `docs/spec-self-contained-audio.md` v0.2 — source of truth;
- `docs/goal-self-contained-audio.md` — concise agent goal;
- `docs/analysis/p1-root-review-amendments.md`;
- `docs/analysis/p2-root-review-amendments.md`;
- `docs/analysis/p3-root-review-amendments.md`;
- `.task-board/EPIC-260712-3agrc1_self-contained-pulsar-audio/plan.md`.

Task board size after root review: 1 Epic, 19 Stories, 205 Tasks. Every Task has
a title, description, scope, acceptance criteria and checklist. `task-board
validate` passes and the Epic dependency graph is acyclic.

## Baseline evidence

- `cd coordinator && go test ./...` — passes.
- `cd pulsar-win && go test ./...` — passes.
- Windows cross-build — passes.
- `cd node-app && swift test` — fails because the current environment cannot
  import module `Testing`; Phase 1 includes a pinned-toolchain repair and does
  not waive this gate.
- Current code has no app-owned microphone capture, generic app upload, common
  media scheduler, Air rooms, streamed user tracks, near-live PTT, capture DSP,
  E2EE media or automation implementation yet.

Only planning/specification/board artifacts have been changed so far. No
feature implementation has started.

## Microsoft Store certification resolution

Product `9P26FDCWV1GC` failed because the reviewer could not exercise primary
functionality without credentials and because screenshots showed only splash or
login surfaces. Phase 1 fixes the product, not merely the reviewer note:

1. accountless launch and local `Try locally`;
2. in-app Create/Join without Spotify or Telegram;
3. local five-second record-then-play plus builtin cue;
4. record, target `This Pulsar`, play and receipt without external accounts;
5. main window, history and settings available on the same path;
6. real RU/EN screenshots of recording, routing, playback and history;
7. certification notes explicitly state that Spotify is optional;
8. current Microsoft policy is re-verified immediately before submission.

The dated policy analysis is
`docs/analysis/store-policy-baseline-2026-07-12.md`. Legal entity, public
contacts, moderation mailbox, hosting and Partner Center authority are real
user inputs and cannot be fabricated.

## Epic execution phases

### Phase 1 — foundations in parallel

- `STORY-260712-2ve1c8` Identity and self-service onboarding.
- `STORY-260712-30ju1k` signed Windows AppContainer platform spike.

The Windows hardware matrix is a hard no-go gate for capture and hotkey work.
Recovery material is shown once; node and control credentials remain separate.

### Phase 2 — canonical media ingest

- `STORY-260712-ld674h` generic media ingest and storage.

Implements bounded resumable upload, hostile-media probing, atomic publication,
target ACL, retention, quota, delete and canonical cancellation interfaces.

### Phase 3 — transmission contract

- `STORY-260712-25lysg` transmission protocol and scheduler.

Freezes target snapshots, prepare/ready/play, coordinator-owned `accepted_at`,
receipts, three-second deadline and
`T = now + max(2*maxRTT + 250 ms, 500 ms)`.

### Phase 4 — policy foundation and mixer in parallel

- `STORY-260712-1tgryz` policy/publication/moderation foundation.
- `STORY-260712-fes2jj` cross-platform overlay and interrupt mixer.

The compliance foundation supplies real policy pages and canonical moderation
before client/bot integration. The mixer preserves main-program timeline,
implements duck/limiter/ceiling ordering and keeps callbacks free of I/O,
allocation and blocking work.

### Phase 5 — Telegram, history and presence

- `STORY-260712-34kbkn` Telegram adapter, history and presence.

Telegram becomes a secure adapter over the same services. Legacy voice defaults
to immediate `after_current`; callbacks are actor-bound, expiring, replay-safe
and cannot replace a started transmission.

### Phase 6 — self-contained desktop UX

- `STORY-260712-2e36uz` main UI, local self-test and capture.

Windows and macOS get accountless Create/Join, explicit microphone permission,
five-second record-then-play, durable unsent drafts, file intake, routing,
receipts, history and tray/menu controls.

### Phase 7 — Phase 1 acceptance and Store resubmission

- `STORY-260712-1i0doc` Store compliance and acceptance.

Independent security, protocol, migration and realtime reviewers plus the root
line review block A1-A8 evidence and Partner Center submission. This is the
hard gate before any Phase 2 task begins.

### Phase 8 — Phase 2 spikes and Air in parallel

- `STORY-260712-3l1r1u` codec/player spike.
- `STORY-260712-3v14m9` Air rooms and approach migration.

The codec ADR must pass exact start ≤5 s, seek ≤3 s, skew ≤100 ms, RSS ≤200 MiB
and duration-independent bounds with version-specific license/SBOM review, or
issue no-go. Air separates saved membership from one active runtime, migrates
approach without dual delivery and supports secure in-app and Telegram control.

### Phase 9 — explicit targets and inbox

- `STORY-260712-ob1tx2` explicit targets, inbox and transport parity.

Extends canonical target snapshots to N recipients, actor-scoped history,
offline inbox, explicit replay, policy consent and report/delete enforcement.
There is never broadcast or autoplay fallback.

### Phase 10 — streamed user tracks

- `STORY-260712-2ori1t` streamed user audio tracks.

Implements variants, authenticated ranges, queue/replace, audible-position
pause/seek/resume, bounded cache and players on both platforms. Combined
Telegram Air/N-target/track parity lives here, after target and Air services.

### Phase 11 — Phase 2 acceptance and seven-day gate

- `STORY-260712-1qfbiw` capacity, migration, rollback and B1-B7 acceptance.

Includes 8-Air/20-node load, every platform pairing, independent codec,
security, migration and realtime reviews, root line review, quota calibration
and seven consecutive real beta days. Critical incidents or unreviewed build
changes reset the seven days. The promotion packet is the hard Phase 3 gate.

### Phase 12 — live PTT and soundboard/automation in parallel

- `STORY-260712-sskhip` near-live PTT.
- `STORY-260712-326wd5` durable soundboard and safe automation.

PTT has separate hold-input and codec/transport spikes, protocol, relay,
per-platform sender/receiver, integration and C1-C2 evidence. Automation has
durable quota-accounted cues, at-most-once schedule claims, IANA/DST/no-catch-up
semantics, one-time scoped tokens, kill switches, desktop UI and Telegram
parity; it can never open a microphone.

### Phase 13 — capture quality and E2EE in parallel

- `STORY-260712-3pt00e` common capture DSP and C3.
- `STORY-260712-1frfmi` client-owned E2EE media.

One DSP serves clips, local self-test and PTT; Windows/macOS probes, deterministic
fixtures and real far/near/double-talk evidence precede claims.

E2EE begins with threat model, audited library and seekable-container spikes,
then protocol and independent design review. Clients own device/group/content
keys; the coordinator only orders signed public membership state and routes
opaque clips, tracks, cues and live frames. There is no silent downgrade,
magical recovery or claim that revoke erases already obtained plaintext.

### Phase 14 — Phase 3 security, beta and release

- `STORY-260712-2ft5wd` integrated C1-C7 and rollout.

Order inside the Story:

1. immutable gate/environment/reviewer matrix;
2. redaction-safe observability;
3. non-delegable root line-by-line implementation review;
4. external crypto plus independent realtime, automation and privacy reviews;
5. final C1-C3, C4-C6 and C7 matrices;
6. rollout/rollback/recovery drills;
7. independent migration/recovery review;
8. seven consecutive real beta days with reset rules;
9. promotion/hold packet;
10. non-delegable root final evidence/release audit.

Promotion is independent for `live_ptt`, `e2ee_media`, `soundboard_cues` and
`automation`. A held capability remains disabled and cannot borrow another
capability's evidence or claims.

## Mandatory review protocol for all future agent code

For every implementation task:

1. implementation agent produces a focused diff and raw test output;
2. task remains unaccepted regardless of agent status or prose;
3. root agent reads the complete diff line by line and checks spec/AC coverage;
4. root reruns relevant tests plus regressions and inspects generated/package
   artifacts and dependency changes;
5. risky seams go to an independent reviewer who did not implement them;
6. findings are fixed and retested on the same commit/build hash;
7. only root review may move the task toward accepted evidence.

No code written after a frozen review or beta hash is silently included. A
relevant change triggers delta review and, where required, beta reset.

## External inputs and honest blockers

- signed AppContainer hardware: real Windows 10 and Windows 11 machines;
- supported macOS hardware, speaker/headphone routes and Bluetooth/USB cases;
- two real home networks and multi-home beta participants;
- qualified independent cryptographic reviewer;
- independent realtime, automation, privacy/Store and operations reviewers;
- real legal/support/moderation contacts and public policy hosting;
- actual Partner Center authority and current Store metadata/screenshots.

Missing inputs block only the affected spike/evidence/release gate. The plan
does not permit fabricated identities, credentials, reviewer sign-off, hardware
results, seven-day logs or external submission.

## Approval gate

Stop here. Do not spawn implementation agents, edit feature code, migrate data,
publish policies, submit to Partner Center or deploy anything until the user
explicitly approves this plan. After approval, begin with Phase 1 task-level
critical paths and preserve the review protocol above.
