# STORY-260712-ld674h decomposition

## Scope anchor

This story covers the common phase-one media ingest and storage substrate behind
Pulsar app uploads and Telegram media:

- common `SubmitMedia(...)` ownership instead of Telegram-only intake;
- authenticated resumable upload sessions;
- authoritative server-side validation with signature probe and `ffprobe`;
- constrained `ffmpeg` normalization to canonical WAV;
- idempotency, tenant-scoped dedupe, quotas, retention, delete and audit;
- legacy WAV and Telegram voice compatibility during mixed rollout.

It does **not** own the UI capture client, self-service control credential
issuance, transmission scheduler semantics, Telegram inline routing UI, or Store
policy surfaces beyond the backend hooks they need.

## Created tasks

1. `TASK-260712-z6h6wh` - Add generic media ingest persistence and migration scaffold
2. `TASK-260712-2af2dp` - Implement SubmitMedia validation and canonical WAV pipeline
3. `TASK-260712-1bnos4` - Add authenticated resumable media upload sessions
4. `TASK-260712-3mcof4` - Enforce media download target ACL
5. `TASK-260712-1sae4q` - Implement media delete, retention and physical cleanup
6. `TASK-260712-gj0cko` - Integrate media ACL, delete and retention lifecycle
7. `TASK-260712-12ojcb` - Move Telegram voice intake onto SubmitMedia without changing legacy behavior
8. `TASK-260712-3huupe` - Add phase-one ingest acceptance and regression coverage
9. `TASK-260712-jolzhh` - Document ingest contract, rollout and cross-story handoff

## Within-story dependency graph

- `TASK-260712-2af2dp` blocked by `TASK-260712-z6h6wh`
- `TASK-260712-1bnos4` blocked by `TASK-260712-z6h6wh`
- `TASK-260712-3mcof4` blocked by `TASK-260712-z6h6wh`,
  `TASK-260712-2af2dp`, and the identity auth foundation
- `TASK-260712-1sae4q` blocked by `TASK-260712-z6h6wh`,
  `TASK-260712-2af2dp`, `TASK-260712-1bnos4`
- `TASK-260712-gj0cko` blocked by `TASK-260712-3mcof4` and
  `TASK-260712-1sae4q`; it integrates their forward-only target/cancellation
  seams with the current runtime without taking ownership of future target rows
- `TASK-260712-12ojcb` blocked by `TASK-260712-2af2dp`
- `TASK-260712-3huupe` blocked by `TASK-260712-2af2dp`, `TASK-260712-1bnos4`, `TASK-260712-gj0cko`, `TASK-260712-12ojcb`
- `TASK-260712-jolzhh` blocked by `TASK-260712-3huupe`

Execution intent:

- Start with schema and repository groundwork.
- Then build the common processing path and app upload session surface in
  parallel.
- Build the target-ACL service and delete/retention worker independently, then
  integrate their fail-closed interfaces and current-runtime cancellation;
  transmission persistence later supplies immutable target rows through them.
- Migrate Telegram after the shared processing path is real.
- Finish with regression coverage, then docs and handoff.

## Cross-story dependencies

- `STORY-260712-2ve1c8` - identity and self-service onboarding
  - Supplies actor model, control-token issuance, secure storage and hashed
    control credentials consumed by upload-session auth.
- `STORY-260712-2e36uz` - main UI, local self-test and capture
  - Consumes the upload-session API and failure semantics defined here.
- `STORY-260712-25lysg` - transmission protocol and scheduler
  - Owns transmission rows, target snapshots, receipts and prepare/play
    semantics. This story must expose ACL hooks and generic media lifecycle
    without duplicating scheduler ownership.
- `STORY-260712-34kbkn` - Telegram adapter, history and presence
  - Owns inline routing actions, human history/presence rendering and broader
    Telegram parity. This story only migrates the ingest path and preserves
    default legacy behavior.
- `STORY-260712-1i0doc` - Store compliance and acceptance
  - Consumes backend delete/report/audit/retention behavior for policy,
    moderation, certification notes and A1-A8 evidence.

## Recommended implementation constraints

- Phase one can use an append-only resumable upload contract with explicit next
  offset tracking. Sparse chunk reassembly is unnecessary for the spec.
- No media becomes `ready` until probe, limits, canonicalization and storage
  metadata are all persisted successfully.
- `ffprobe` and `ffmpeg` process untrusted bytes with network protocols disabled,
  fixed arguments, and CPU, memory, time, and output-size caps. Canonical bytes
  are published atomically; stale workers cannot overwrite terminal state.
- Deduplication is scoped to one orbit only. Cross-orbit shared hashes must not
  reveal existence.
- Delete must revoke new fetches immediately even if physical byte cleanup is
  deferred.
- Sender delete follows one frozen policy for queued, prepared, scheduled, and
  already-playing media; clients and moderation may not invent different rules.
- Telegram continues to enqueue default voice by coordinator acceptance time,
  not by processing completion time.

## Completeness check against story AC

- Phase-one input formats and limits from spec sections 7-8 are covered by
  `TASK-260712-2af2dp` and proven by `TASK-260712-3huupe`.
- Interrupted and retried uploads without duplicates are covered by
  `TASK-260712-1bnos4` plus end-to-end coverage in `TASK-260712-3huupe`.
- Corrupt, oversized and timed-out inputs never becoming ready are enforced in
  `TASK-260712-2af2dp` and regression-tested in `TASK-260712-3huupe`.
- Tenant ACL and deletion protections are implemented in
  `TASK-260712-gj0cko` and verified in `TASK-260712-3huupe`.
- Existing Telegram voice order and output compatibility are preserved in
  `TASK-260712-12ojcb` and regression-tested in `TASK-260712-3huupe`.

## Workflow note

The board instance rejected child-task status promotion while tasks are
unassigned. The tasks remain unassigned with full scope, AC, checklists,
dependencies and linked diagrams; they are ready for assignment and developer
pickup.
