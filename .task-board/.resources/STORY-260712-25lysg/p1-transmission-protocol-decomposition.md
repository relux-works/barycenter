# P1 Transmission Protocol and Scheduler Decomposition

Story: `STORY-260712-25lysg`

## Task set

1. `TASK-260712-51y5k9` Clarify transmission status, DND, and downgrade contracts
   - Explicit blocking task because the source-of-truth defines behavior and
     example payloads, but it leaves the exact HTTP response fields, DND or
     presence payloads, and visible downgrade contract implicit.
2. `TASK-260712-1aprcb` Add transmission persistence, target snapshots, and ACL enforcement
   - Additive schema, receipt state storage, block data, and target-snapshot
     media authorization.
3. `TASK-260712-2qpp6w` Implement transmission HTTP API and audience resolution
   - Control-token endpoints, explicit target resolution, downgrade recording,
     and cancel/status behavior.
4. `TASK-260712-1g70av` Land clip transmission wire contract and capability negotiation
   - Golden-backed protocol additions across Go, Windows, and Swift.
5. `TASK-260712-31vvjt` Implement coordinator overlay controller and prepare barrier scheduler
   - Separate scheduler state, prepare barrier, partial readiness, receipts,
     cancel, and legacy after-current bridge.
6. `TASK-260712-2bbz13` Add Windows clip transmission client and receipt hooks
   - Prepare, ready, scheduled play, cancel, failure, and presence plumbing on
     the Windows node while preserving legacy voice.
7. `TASK-260712-26ip33` Add macOS clip transmission client and receipt hooks
   - Prepare, ready, scheduled play, cancel, failure, and presence plumbing on
     the macOS node while preserving legacy voice.
8. `TASK-260712-2qc27p` Add transmission compatibility, ACL, and scheduler regression coverage
   - Final proof for ordering, receipts, ACL, downgrade, migration, and
     cross-codec compatibility.
9. `TASK-260712-2cdjq8` Document transmission contract, rollout, and cross-story handoff
   - Final contract summary, rollout order, and downstream handoff.

## Execution shape

- Blocking clarification: `TASK-260712-51y5k9`
- Parallel after clarification: `TASK-260712-1aprcb` and
  `TASK-260712-1g70av`
- API path: `TASK-260712-51y5k9` + `TASK-260712-1aprcb` ->
  `TASK-260712-2qpp6w`
- Runtime path: `TASK-260712-1aprcb` + `TASK-260712-2qpp6w` +
  `TASK-260712-1g70av` -> `TASK-260712-31vvjt`
- Node client path: `TASK-260712-1g70av` ->
  `TASK-260712-2bbz13` and `TASK-260712-26ip33`
- Final proof: store + API + wire + scheduler + both client tasks ->
  `TASK-260712-2qc27p`
- Handoff and rollout note: `TASK-260712-2qc27p` ->
  `TASK-260712-2cdjq8`

## Completeness check

- Covered:
  - transmission schema, target snapshots, receipts, and block data
  - HTTP create/status/cancel endpoints and audience resolution
  - WebSocket protocol additions and capability negotiation in all three codecs
  - separate overlay controller, prepare barrier, partial readiness, and FIFO
    ordering
  - Windows and macOS node hooks for prepare, ready, scheduled play, cancel,
    failure, and legacy compatibility
  - target-snapshot media ACL and migration safety
  - regression, compatibility, rollout, and downstream documentation
- Explicit gap closed with blocker:
  - exact transmission status, DND or presence, cancel, and visible downgrade
    contracts
- Diagrams attached:
  - `p1-transmission-protocol-components.puml`
  - `p1-transmission-scheduler-sequence.puml`
