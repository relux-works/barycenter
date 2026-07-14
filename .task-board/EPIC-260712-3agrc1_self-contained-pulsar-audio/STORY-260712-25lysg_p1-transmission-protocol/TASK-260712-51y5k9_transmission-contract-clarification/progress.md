## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T07:26:35Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1aprcb
- TASK-260712-2qpp6w
- TASK-260712-1g70av
- TASK-260712-1hqiek
- TASK-260712-1c1ska
- TASK-260712-31rkpe

## Checklist
- [x] Write the transmission HTTP and WebSocket contract note
- [x] Define visible downgrade and cancel semantics
- [x] Define DND and block ownership boundaries for downstream stories
- [x] Freeze origin defaults, delivery-kind rules, overlay limit, trusted accepted_at and stale-play behavior
- [x] Freeze the exact barrier formula and whole-transmission mixed-fleet downgrade rule
- [x] Separate visible overlay downgrade from interrupt fallback that requires explicit user confirmation
- [x] Freeze sender-delete behavior and receipts for queued, prepared, scheduled and already-playing media

## Notes
Strict sequential inline execution started 2026-07-14 from clean main merge c4cb324bb4e783e97bb1fbf1bb61efef9dfbf10f after TASK-260712-jolzhh and the full P1 media ingest story were accepted. Scope is contract clarification and durable documentation; implementation remains in the ordered downstream tasks. Manual real-app and physical-hardware evidence stays in EPIC-260714-th54l3.
Normative p1-transmission-v1 contract drafted and linked from the source spec and protocol notes. It freezes strict HTTP shapes, immutable audience and visibility rules, trusted acceptance ordering, origin defaults, target and aggregate enums, whole-overlay downgrade, explicit interrupt confirmation, five-minute speak-now expiry, exact three-second barrier and RTT formula, 100 ms stale-play rejection, generation-safe wire payloads, DND/block ownership, and click-free active delete. Added a Go guard that parses all 23 JSON examples and pins critical decisions. Focused protocol and race tests, full coordinator tests/vet, Windows portable tests, diff checks and board validation pass. No real-app or physical-hardware evidence is claimed; that remains in EPIC-260714-th54l3.
Self-review delta closed: terminal aggregate reason_code values are now deterministic across API/history/bot; cancel-vs-start is resolved by writer commit order with race-safe fade_stop/resume_once; register capability replacement and DND revision acknowledgement are explicit; expired/deleted media keeps the existing non-oracular 404 boundary; speak-now fallback retains a five-minute intent expiry. Outcome resource refreshed byte-for-byte and the JSON/decision guard remains green.
Hosted CI run 29314060965 passed coordinator (including pinned previous-head compatibility and the contract guard), authoritative macOS NodeCore tests, portable Windows tests/cross-build, and the signed packaged Windows probe on implementation commit 605859b. Review verdict: accepted; no manual real-app/hardware claim.
Delta review from downstream persistence task corrected the logical spec example prefix md_ to the already-shipped and test-pinned generic media ID m_<ULID>. This is a documentation correction, not a schema migration or behavior change; changing production IDs would regress the accepted ingest story.

## Precondition Resources
- [p1-transmission-scheduler-sequence.puml](file://TASK-260712-51y5k9/p1-transmission-scheduler-sequence.puml) — Clarification diagram for transmission flow, receipts, and legacy downgrade

## Outcome Resources
- [TASK-260712-51y5k9_transmission-contract-v1.md](file://TASK-260712-51y5k9/TASK-260712-51y5k9_transmission-contract-v1.md) — Normative Phase 1 HTTP, WebSocket, scheduler, DND, downgrade, cancel and delete contract
