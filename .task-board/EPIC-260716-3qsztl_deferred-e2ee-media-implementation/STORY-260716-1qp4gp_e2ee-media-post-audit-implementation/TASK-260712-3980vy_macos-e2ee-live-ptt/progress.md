## Status
done

## Assigned To
[reviewer] reviewer (claude)

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-20T04:01:43Z

## Blocked By
- TASK-260712-1x9ruo
- TASK-260712-1yz5ca
- TASK-260712-2kj9kj
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2nppt6
- TASK-260712-1bcpda

## Checklist
- [x] Derive a unique session key and bind all live context into AAD
- [x] Encrypt sender frames off capture callbacks and verify before jitter decode
- [x] Reject nonce reuse replay tamper stale epoch and removed sender
- [x] Preserve C1 C2 FEC PLC backpressure DND and teardown bounds
- [x] Prove coordinator traffic capture cannot reproduce macOS speech
- [x] Before runtime wiring enforce single-instance ownership of MacE2EEKeyStateRepository or add cross-process serialization so send generations cannot be double-reserved.
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [x] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
2026-07-20 producer implementation is production-dark: exact BE opaque wire mirror, witnessed live_ptt generation reservation and provider derivation seam, AAD binding, retry-safe sealing, auth-before-jitter, replay/nonce/membership teardown, 8 focused Swift tests, 348 full Swift tests, 200 acceptance discovery tests and 16/16 harness. Audit transform only; no production provider/runtime/capability/UI claim. Real C1-C2, packet capture, signed package, memory/crash, cross-process contention and macOS-Windows interop remain manual under TASK-260712-flaiie and TASK-260712-yj668d in EPIC-260714-th54l3. Production constructor remains gated on reviewed provider plus explicit cross-process generation serialization approval.
spawn queued: [reviewer] reviewer (claude) (run=RUN-260720-db683a, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260720-db683a)
2026-07-20 independent review of d8a429c: REJECTED -> to-dev. Evidence battery reproduced green (lint clean, 8/8 focused, 348/348 full Swift, 200/200 acceptance discovery, 16/16 harness, all pinned hashes match); BE wire is byte-for-byte with the Go authority, fail-closed/realtime/production-dark posture sound. One HIGH: MacE2EELiveSessionContext.groupRevision binds the device-LOCAL Keychain record revision (persist = expectedRevision+1; reserveSendGeneration bumps only the sender) into AAD, while prepareIncoming forces the receiver to bind its own local revision — two distinct installations can essentially never compute equal AAD, so every cross-device frame fails authentication from frame 1; loopback tests share one context and mask it (factory test hand-tunes revision+1). Fail-closed, no security exposure, but the core bridge AC (all macOS source/target pairings) is unachievable outside loopback. Fix: bind shared witnessed values (epoch + commitDigest, or a truly synchronized group revision) instead of the local lineage counter; keep local-revision checks only in the witnessed re-read; add a two-installation protect->open round-trip fixture; treat the AAD change as a cross-client contract delta for the audit gate. Also LOW: malformed provider output misreported as nonceReuse; seal invoked before sequence<=15000 encodability check at the duration boundary. Details in TASK-260712-3980vy_review-verdict-v1.md.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260720-db683a, pid=51768, exit=0)
2026-07-20 independent run RUN-260720-db683a rejected producer d8a429c on HIGH-1: device-local Keychain record revision was incorrectly included in cross-device AAD, so sender reservation revision and receiver local revision diverged. Rework replaces it with shared witnessed commitDigest while retaining local revision only for setup CAS, adds two-installation skewed-revision protect/open coverage, separates malformed provider output from nonce reuse, and checks sequence <=15000 before sealing. Re-review required on exact rework SHA; the AAD delta is recorded for independent delta review even though BE wire is unchanged.
spawn queued: [reviewer] reviewer (claude) (run=RUN-260720-8f681f, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260720-8f681f)
2026-07-20 independent re-review of rework c9faa7e (run RUN-260720-8f681f): ACCEPTED -> done. HIGH-1 closed: device-local Keychain record revision fully removed from cross-device surface; AAD and per-frame authorization now bind shared witnessed epoch + commitDigest (installed only via persistGroupState epoch-transition validation; reserveSendGeneration provably never mutates it), local revision retained only as setup/CAS witnesses (expectedGroupRevision / expectedLocalGroupRevision). New crossInstallationRoundTrip fixture uses two separate repositories with deliberately skewed local revisions (2 vs 1 asserted) and proves byte-identical AAD via protect->open round-trip — d8a429c fails this test. LOW-1 closed: malformedProviderOutput is a distinct fail-closed terminal classification in both directions. LOW-2 closed: sequence<=15000 enforced before AAD/seal; fixture proves sealCount stays 15000 at attempt 15001 with non-terminal invalidFrame and no nonce/state growth. AAD delta properly recorded as cross-client contract change (ADR, vectors commit_digest/changed_commit_digest, renamed packet invariant, validator source+fixture enforcement, deltaReviewRequired=true) while public BE wire stays byte-identical (opaque frame code untouched, vector encodedHex unchanged). No new Critical/High/Medium; v1 INFO-1..3 remain non-blocking. Evidence independently reproduced at c9faa7e: swift-format strict clean, focused 10/10, full Swift 350/350, acceptance discovery 200/200, automated harness 16/16, all 9 packet hash pins match, no sibling cascade needed (MacE2EEKeyState.swift pin unchanged), NodeApp grep clean, production darkness intact. No production E2EE readiness claimed; manual hardware/real-provider work remains in EPIC-260714-th54l3. Details in TASK-260712-3980vy_review-verdict-v2.md.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260720-8f681f, pid=82365, exit=0)

## Precondition Resources
- [independent-review-brief.md](file://TASK-260712-3980vy/independent-review-brief.md) — Exact d8a429c independent security/protocol/realtime review instructions
- [independent-re-review-brief-c9faa7e.md](file://TASK-260712-3980vy/independent-re-review-brief-c9faa7e.md) — Exact c9faa7e closure review for HIGH-1 and Low findings

## Outcome Resources
- [TASK-260712-3980vy_spawn-log_-reviewer--reviewer--claude-.log](file://TASK-260712-3980vy/TASK-260712-3980vy_spawn-log_-reviewer--reviewer--claude-.log) — System spawn log captured by task-board
- [TASK-260712-3980vy_review-verdict-v1.md](file://TASK-260712-3980vy/TASK-260712-3980vy_review-verdict-v1.md) — Independent review verdict for d8a429c: REJECTED with one High finding (device-local record revision bound into AAD breaks cross-device authentication), 2 Low, 3 informational; full evidence battery reproduced green
- [TASK-260712-3980vy_review-verdict-v2.md](file://TASK-260712-3980vy/TASK-260712-3980vy_review-verdict-v2.md) — Independent re-review verdict for rework c9faa7e: ACCEPTED — HIGH-1, LOW-1, LOW-2 closed, no new Critical/High/Medium, full evidence battery reproduced green
