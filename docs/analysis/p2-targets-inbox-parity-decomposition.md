# P2 Explicit Targets, Inbox, and Transport Parity Decomposition

Story: `STORY-260712-ob1tx2`

## Current implementation seams

- `coordinator/internal/store/store.go`
  - `GetMediaForOrbit` still authorizes media by owner-or-active-approach,
    not by immutable target snapshots.
- `coordinator/internal/session/fsm.go`
  - `targetNodes` still models delivery as one slot or `both`.
- `coordinator/cmd/duet-coordinator/loop.go`
  - `injectTargets` and `processMediaDone` still encode Telegram-specific
    personal delivery and silently broadcast when there are more than one
    recipient candidates.
- `coordinator/internal/bot/commands.go`
  - Telegram still exposes queueing and targeting directly instead of acting as
    a thin adapter over one transport-neutral application service.
- `coordinator/cmd/duet-coordinator/main.go`
  - `/media/{id}` still serves by orbit/link membership rather than the exact
    accepted target set.

## Task set

1. `Freeze inbox, replay, and parity contracts`
   - Blocking clarification for the exact request and response contract for
     explicit targets, inbox entries, pagination cursors, replay or delete
     authorization, mixed-version unsupported-target exposure, and parity
     rules shared by Pulsar and Telegram.
2. `Persist target snapshots, inbox rows, and ACL-safe receipt history`
   - Add the additive schema and repository layer for immutable explicit target
     snapshots, inbox lifecycle rows, paginated receipt reads, and fetch
     revocation tied to delete or disable decisions.
3. `Implement common explicit-target transmission and replay services`
   - Replace one-or-both targeting with transport-neutral N-recipient target
     resolution, include-origin policy, explicit replay creation, no-broadcast
     fallback, and coordinator-owned delivery decisions for clip and track
     actions.
4. `Expose inbox, receipt pagination, and replay or delete APIs`
   - Add the caller-facing query and mutation surfaces for history, inbox,
     receipts, replay, delete, and capability-aware unsupported-target status.
5. `Enforce rights, reports, and disable-driven fetch revocation`
   - Gate file upload on content-policy acceptance and make report, delete,
     actor disable, and orbit disable actually stop future fetch and replay.
6. `Bring Telegram onto the common parity service`
   - Route Telegram voice, audio, and document actions through the same common
     service with identical queue or replace or target semantics and identical
     human-readable errors.
7. `Add Pulsar inbox and explicit-target history surfaces`
   - Desktop history, inbox, receipt pagination, manual replay, delete, report,
     mute, and unsupported-target status using the same domain contract as
     Telegram.
8. `Prove B5-B7 with ACL, mixed-version, inbox, and abuse regressions`
   - Final evidence for API/UI invisibility of non-targets, no late autoplay,
     N-recipient personal delivery, mixed-version explicit policy, and rights
     enforcement at fetch boundaries.
9. `Document rollout, migration order, and downstream handoff`
   - Final implementation note for deploy order, mixed-version window, and the
     contract other phase-two stories consume.

## Execution shape

- Blocking clarification: task 1
- Parallel data and contract foundation: tasks 2 and 3 after task 1
- Query and mutation surface: tasks 2 + 3 -> task 4
- Rights enforcement: tasks 1 + 2 + 4 -> task 5
- Transport adapters:
  - task 4 + task 5 -> task 6
  - task 4 + task 5 -> task 7
- Final proof: tasks 2 through 7 -> task 8
- Handoff and rollout note: task 8 -> task 9

## Completeness check

- Covered:
  - immutable explicit target ACL snapshots
  - N-recipient personal delivery without broadcast fallback
  - inbox rows, TTL, replay, delete, and no-late-autoplay semantics
  - receipt and history pagination
  - Telegram parity for voice, audio, and document actions
  - shared Pulsar and Telegram error or status semantics
  - content-policy acceptance, report, delete, and disable enforcement
  - mixed-version unsupported-target visibility needed by `B6`
- Explicit gap closed with blocker:
  - exact transport-neutral contract for inbox, replay, pagination, mixed
    versions, and delete or disable side effects
- Diagrams attached:
  - `p2-targets-inbox-parity-components.puml`
  - `p2-targets-inbox-parity-sequence.puml`
