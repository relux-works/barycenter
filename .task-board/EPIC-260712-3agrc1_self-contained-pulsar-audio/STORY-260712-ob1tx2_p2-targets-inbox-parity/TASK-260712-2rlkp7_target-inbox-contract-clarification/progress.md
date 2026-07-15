## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:51Z

## Last Update
2026-07-15T22:01:02Z

## Blocked By
- TASK-260712-1xik11

## Blocks
- TASK-260712-2bk0vy
- TASK-260712-1c34fe
- TASK-260712-2j5fkr
- TASK-260712-2zoy4u
- TASK-260712-2ctf3x
- TASK-260712-31rkpe

## Checklist
- [x] Write the common create status replay and delete contract
- [x] Freeze inbox entry fields pagination cursors and receipt aggregates
- [x] Define mixed version unsupported target exposure and parity rules
- [x] Define delete disable and no late autoplay side effects
- [x] Freeze targeted-track behavior and non-target node state inside one Air
- [x] Freeze inbox target identity, eligible missed reasons, TTL, replay lineage and zero-autoplay behavior
- [x] Reuse Phase 1 ACL, history, secure callbacks and moderation instead of defining parallel models
- [x] Freeze anti-denial-of-service report versus quarantine versus delete or disable behavior

## Notes
2026-07-15 strict-sequence start from synchronized main a89dd9e. Implementing inline outside task-board spawn workflow. The Phase 2 contract will extend the existing p1-transmission-v1 immutable target snapshots, p1 history cursors, secure action callbacks, media ACL and moderation evidence. It will freeze N-recipient explicit targets, audio-track targeting, inbox eligibility/TTL/manual replay, mixed-version visibility and report/quarantine/delete distinctions without creating a second ACL, history or Telegram model.
Accepted after engineering PR #120 merged as 100678f (head 22e2aa4). The p2-targets-inbox-parity.v1 contract extends Phase 1 immutable target snapshots, history cursors, media ACL, secure callbacks and moderation authority; parallel ACL/history/moderation and Telegram-owned queues are forbidden. It freezes atomic N-recipient create, targeted audio_track queue/replace without broadcast fallback, fail-closed mixed-version 422 without partial rows, 9 eligible missed reasons, 30-day bounded TTL, exact inbox fields and cursor bindings, manual replay as a new transmission with depth-8 lineage, sender delete versus local dismiss, consent reauthorization and zero late autoplay. Reports have reporter-local effects only; reversible audited operator quarantine is distinct from terminal delete/disable and cannot be triggered by report count. HTTP, Windows, macOS and Telegram share fields, enums, auth and errors. Validator plus negative tests passed inside 31/31 contract tests; hosted CI run 29453630078 passed all four jobs.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-sequence.puml](file://TASK-260712-2rlkp7/p2-targets-inbox-parity-sequence.puml) — Common explicit-target, missed-delivery, report and manual-replay sequence
- [targets-inbox-contract-v1.json](file://TASK-260712-2rlkp7/targets-inbox-contract-v1.json) — Normative machine-readable targets, inbox, replay and parity contract
- [p2-targets-inbox-contract-v1.md](file://TASK-260712-2rlkp7/p2-targets-inbox-contract-v1.md) — Normative contract narrative and downstream implementation boundary
- [validate_targets_inbox_contract.py](file://TASK-260712-2rlkp7/validate_targets_inbox_contract.py) — Fail-closed contract validator
- [hosted-coordinator-manifest.json](file://TASK-260712-2rlkp7/hosted-coordinator-manifest.json) — Hosted run 29453630078 coordinator acceptance manifest
- [hosted-swift-manifest.json](file://TASK-260712-2rlkp7/hosted-swift-manifest.json) — Hosted run 29453630078 Swift acceptance manifest
- [hosted-windows-manifest.json](file://TASK-260712-2rlkp7/hosted-windows-manifest.json) — Hosted run 29453630078 Windows acceptance manifest
