## Status
development

## Assigned To
codex-root-inline

## Created
2026-07-12T16:40:36Z

## Last Update
2026-07-20T09:11:21Z

## Blocked By
- TASK-260712-20j5tm
- TASK-260712-1rziyo
- TASK-260712-2i0w6x
- TASK-260712-2nppt6
- TASK-260712-2q4jbu
- TASK-260712-1yz5ca
- TASK-260712-39vjzd
- TASK-260712-3980vy
- TASK-260712-aniuyy

## Blocks
- TASK-260712-yj668d
- TASK-260712-1ulshp

## Checklist
- [ ] Build revoked-member, new-member, history-grant, and privacy regression coverage.
- [ ] Capture storage or traffic unreadability and honest metadata-disclosure evidence.
- [ ] Include cross-platform, mixed-version, rollback, and recovery or transfer cases.
- [ ] Publish the external review packet with residual risks and the required closure step.
- [ ] Freeze feature-flag and claim handoff to the acceptance or rollout story.

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
2026-07-16 E2EE split: this task owns the root-reviewed implementation evidence and code-review packet consumed by TASK-260712-1ulshp inside EPIC-260716-3qsztl. It does not block the non-E2EE final review cycle in EPIC-260712-3agrc1.
2026-07-20 strict sequential execution started on branch feat/task-260712-1bcpda from accepted main merge 9d7ace6dc7337cd2191f35b0d8373228cf759398. Engineering-only scope: build a reproducible source-linked C4-C6 implementation evidence and external-review handoff packet over all accepted production-dark E2EE components; add deterministic known-answer/malformed/state/privacy/rollback/mixed-version regression coverage where repository-executable; freeze hashes, claims, residual risks and e2ee_media disabled posture. Real Windows-Windows/Windows-macOS/macOS-macOS packaged-app, storage/traffic capture, OS secure-storage, audio, accessibility and physical recovery evidence stays not-run in manual TASK-260712-yj668d under EPIC-260714-th54l3. This task does not self-certify external TASK-260712-1ulshp. Independent Claude Fable 5 max exact-SHA review with zero open Critical/High/Medium is required before engineering acceptance.

## Precondition Resources
- [p3-e2ee-media-sequence.puml](file://TASK-260712-1bcpda/p3-e2ee-media-sequence.puml) — Validation sequence for C4-C6 proof, history grants, revoke rotation, and report evidence

## Outcome Resources
(none)
