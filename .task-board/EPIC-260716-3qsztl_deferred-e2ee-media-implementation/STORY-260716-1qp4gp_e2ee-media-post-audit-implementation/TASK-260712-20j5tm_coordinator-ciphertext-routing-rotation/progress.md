## Status
done

## Assigned To
[reviewer] reviewer (claude)

## Created
2026-07-12T16:40:33Z

## Last Update
2026-07-19T20:18:10Z

## Blocked By
- TASK-260712-2ys1ww
- TASK-260712-3w1cst
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2i0w6x
- TASK-260712-2nppt6
- TASK-260712-2q4jbu
- TASK-260712-1bcpda
- TASK-260712-1yz5ca
- TASK-260712-25dzp4
- TASK-260712-1x9ruo
- TASK-260712-1rziyo

## Checklist
- [x] Replace plaintext protected-media routing with manifest and ciphertext handling behind the feature flag.
- [x] Seal recipient snapshots to epochs and rotate on join, leave, revoke, and Air changes.
- [x] Reuse ACL, delete, retention, and inbox or history services without plaintext regressions.
- [x] Reject unsupported or stale-epoch fetches explicitly in mixed-version tests.
- [x] Keep feature-off legacy behavior intact.
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [x] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
Root-reviewed correction: rotate means serialize and route client-produced signed group commits. Coordinator must never create unwrap escrow or log group or content secrets. Original checklist wording is subordinate to this invariant.
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
Execution started 2026-07-19 on branch feat/task-260712-20j5tm from merged schema foundation 2ab8a135. Scope remains feature-off and production-dark; independent delta review required before acceptance.
Producer implementation in progress: additive protocol-actor bindings, exact group-member snapshots, rotation requirements, durable event deliveries/acks, strict injected-verifier proposal/commit routing, and protected-write rotation gate are implemented production-dark. Focused and focused-race suites pass; full coordinator + go vet pass. Evidence packet and independent delta review remain pending.
Producer verification complete before commit: focused tests, full coordinator go test ./..., go vet ./..., 207/207 acceptance-contract checks, targeted evidence validators, and full race go test -race ./internal/store ./internal/e2eecontract -count=1 passed (store 502.193s; contract 1.427s). Evidence hashes reproduce; independent review and exact producer SHA still pending.
spawn queued: [reviewer] reviewer (claude) (run=RUN-260719-47433f, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260719-47433f)
Independent delta review APPROVE at exact producer SHA e97717bfad6348279430012ecf0ce3de404eae0d. Zero open Critical/High/Medium delta findings. Reproduced: 12/12 packet SHA-256 pins MATCH; coordinator go test ./... + go vet clean; race store 525.144s / e2eecontract 1.446s; focused routing tests all PASS; validate_e2ee_coordinator_routing_rotation.py PASS; acceptance-contract 207/207 OK. Coordinator confirmed keyless (empty-suite+nil-verifier ProductionConfig rejects even valid fixture; strict forbidden/unknown-field decoders; no secret columns; dormant/no runtime wiring; e2ee_media_v1 absent from production sources). Exact device/actor/Air lineage bound into canonical snapshot; join/leave/same-device-rejoin/role/device-revoke/actor-disable each rotate; staging double-checks membership inside the write tx (no race past a changed snapshot); single-winner commit CAS + partial unique index; fork freeze on mismatched predecessor; malformed predecessor does not poison; durable restart delivery + collision-safe cursor + exact digest/revision ack + bounded audit. Additive-only (4 new tables, feature lock enabled=0 intact, no legacy ACL/delete/retention/inbox/history DML); opaque-media runtime deferred to named tasks = documented deferral, not an acceptance gap. Windows/macOS suites not run: delta touches no pulsar-win/node-app file. Non-blocking: L1 multi-cause rotation reason_code non-deterministic (audit-fidelity only); I1 member-with-only-revoked-devices semantics to pin downstream (EPC-005). Verdict resource: TASK-260712-20j5tm_independent-delta-review-v1.md. Production gates EPC-001/002/004/005 + TASK-260712-1ulshp remain open by design.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260719-47433f, pid=17703, exit=0)

## Precondition Resources
- [p3-e2ee-media-components.puml](file://TASK-260712-20j5tm/p3-e2ee-media-components.puml) — Coordinator ciphertext-routing and rotation boundary diagram
- [p3-e2ee-media-sequence.puml](file://TASK-260712-20j5tm/p3-e2ee-media-sequence.puml) — Protected-send and revoke-rotation sequence for coordinator runtime
- [independent-delta-review-brief.md](file://TASK-260712-20j5tm/independent-delta-review-brief.md) — Exact-SHA independent reviewer scope and evidence challenge

## Outcome Resources
- [TASK-260712-20j5tm_spawn-log_-reviewer--reviewer--claude-.log](file://TASK-260712-20j5tm/TASK-260712-20j5tm_spawn-log_-reviewer--reviewer--claude-.log) — System spawn log captured by task-board
- [TASK-260712-20j5tm_independent-delta-review-v1.md](file://TASK-260712-20j5tm/TASK-260712-20j5tm_independent-delta-review-v1.md) — Independent delta review verdict (APPROVE) at exact producer SHA e97717b — reproduced hashes, tests, race, 207/207 acceptance
