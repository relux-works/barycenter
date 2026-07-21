## Status
development

## Review
required

## Task Class
code

## Blocked By
- TASK-260721-8pwqje

## Blocks
- TASK-260721-ryk8c0

## Checklist
- [x] Run and manifest every deterministic Windows and macOS UI/package verification command.
- [ ] Verify exact hashes and publish remaining limitations without manual pass claims.
- [ ] Update the single Ivan Oparin owner task with one concise real-app checklist.

## Notes
2026-07-21 strict execution started after macOS candidate b0cc1a2 passed 359 Swift tests, release build, and independent Claude Opus 4.8 review RUN-260721-2dfdc7. Fable 5 run RUN-260721-ad6a04 returned provider 429 before tokens and is not a verdict. This task will publish one automated exact-head manifest and one consolidated Ivan Oparin real-app checklist without manual pass claims.
2026-07-21 clean local acceptance on exact a108080 passed 11/11 stages: desktop contracts; Windows vet/test/race/cross-vet/cross-build; amd64+arm64 GUI builds with PE subsystem=2; Swift 359-test suite; Swift release build. Manifest SHA-256 cc555975142c13d02490f723f83aa7b61cd7288604efdf593cc600b6c1317b7e records git tree, pinned Go/Xcode versions, 15 source hashes, three build hashes and manualEvidence=not-run. Superseded run 0c4db88 failed closed because it modified the frozen shared E2EE acceptance runner; a108080 restores that runner byte-for-byte and isolates desktop acceptance. Exact-head hosted CI run 29828164444 is queued.
Hosted CI 29828164444 proved both Windows jobs and signed package job, but its macOS job reproduced five pre-existing MacLivePTTNodeTests scheduler timeouts under full-suite parallel startup. This is treated as a concrete test-infrastructure defect, not rerun away. Candidate c60bbec serializes that six-test suite and raises its bounded private-queue wait from 2s to 10s; focused suite passed 5/5 repeated runs. A new clean desktop manifest and exact-head hosted CI are required before acceptance.

## Precondition Resources
(none)

## Outcome Resources
- [desktop-ui-a108080-manifest.json](file://TASK-260721-2346wf/desktop-ui-a108080-manifest.json) — Clean exact-head desktop UI automated acceptance manifest: 11/11 commands passed, source and build hashes recorded, manual evidence not run

## Created
2026-07-21T10:56:33Z

## Last Update
2026-07-21T12:04:02Z

## Assigned To
codex-root-inline
