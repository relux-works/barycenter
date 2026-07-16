## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:37:28Z

## Last Update
2026-07-16T20:58:32Z

## Blocked By
- TASK-260712-3a0cf9

## Blocks
- TASK-260712-3sv87k
- TASK-260712-1kk8bd
- TASK-260712-1eva0y
- TASK-260712-11e4e3
- TASK-260712-hb5xz2

## Checklist
- [x] Decide and record the supported automation entry point and why it is safe under the specification threat model.
- [x] Freeze cue-class media scope and explicit rejection of microphone or long-track automation in this story.
- [x] Freeze target selectors, DND and quiet-hour precedence, and exact denied reasons.
- [x] Freeze scoped-principal issuance, revoke, disable, and attribution fields for history and audit.
- [x] Record mixed-version and feature-flag behavior for unsupported clients or disabled automation.

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge bb023c8c48d7b63ab3a5f42af16d0d5f3b59e0a1. Execute inline outside task-board spawn. Freeze the threat-modeled cue-only automation surface before schema/runtime work; preserve hard no-microphone, no-long-track and fail-closed mixed-version boundaries.
2026-07-16 accepted after root line review. Chose coordinator authenticated HTTPS POST /v1/automation/triggers; no webhook or loopback listener. Frozen audio_clip/hash-pinned builtin-only cues, no microphone/voice/track/live path, overlay-only automation, least-privilege hashed principals, exact audience/DND/quiet-hour/DST/no-catch-up/revoke/rate/mixed-version rules. Code commit c9b8e551c4877350e450f809524c52a2fe1bb8ce; PR #203; hosted CI 29533919029 passed 4/4; clean acceptance 12/12 at /private/tmp/barycenter-accept-c9b8e55/.temp/acceptance/20260716Tautomationcontract/manifest.json with manualEvidence=not-run. Merged to main as 4d2fa559b5ceb818ff239e36495c53bc5f841b30. Real app/hardware stays in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p3-soundboard-automation-decomposition.md](file://TASK-260712-3sj8ox/p3-soundboard-automation-decomposition.md) — Contract-gap context and dependency map for the automation surface blocker task
- [p3-soundboard-automation-components.puml](file://TASK-260712-3sj8ox/p3-soundboard-automation-components.puml) — Component diagram for automation surface, control APIs, runtime, and prerequisite seams
- [p3-automation-safety-contract-v1.md](file://TASK-260712-3sj8ox/p3-automation-safety-contract-v1.md) — Normative v1 threat model, cue/principal/quiet-hours contract and executable handoff
