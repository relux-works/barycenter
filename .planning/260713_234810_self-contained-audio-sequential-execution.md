# Sequential execution plan: Self-contained Pulsar Audio

- Date: 2026-07-13
- Epic: `EPIC-260712-3agrc1` — Self-contained Pulsar Audio
- Baseline: `main` at merge commit `0c75dcfff601379f8fc53a18034b5a0530fdcb2b` (PR #8)
- Inventory: 19 stories, 205 tasks; 6 accepted, 3 checkpointed, 196 not started.

## Operating contract

This is a standalone execution plan. Task-board IDs are retained only as stable
links to the existing task briefs, acceptance criteria and evidence; execution
does not use task-board status transitions, assignment, spawn or review workflow.

Execute exactly one unchecked task at a time, from top to bottom. Do not begin
the next task until the current task's implementation, tests, evidence and
required review are accepted and landed on `main`. Within a task:

1. start from a clean, current `origin/main` and read the task README plus its
   precondition resources and the authoritative specification;
2. freeze the intended scope and acceptance matrix before editing;
3. implement the smallest complete change, with focused tests and regression
   coverage; do not mix unrelated cleanup into the task;
4. run platform-relevant tests and inspect generated, packaged and migration
   artifacts rather than relying on a green command alone;
5. independently review security, protocol, migration, realtime-audio and
   release-gate work; resolve findings on the reviewed bytes;
6. record exact commands, raw results, artifact/build hashes, known limitations
   and rollback instructions;
7. land the accepted change before checking the item and moving to the next one.

If a task needs real hardware, signing identity, Partner Center authority,
legal/support data, public hosting, external reviewers or elapsed beta time,
stop at that task. Do not count a mock, developer-mode path, fabricated input or
unreviewed build as evidence and do not skip ahead to a dependent task.

## 0. Landed and accepted baseline — do not reimplement

- [x] `TASK-260712-1bpog0` — identity-schema-auth-foundation
- [x] `TASK-260712-3v1k7q` — recovery-link-contract-clarification
- [x] `TASK-260712-2xkyot` — telegram-link-legacy-migration-compat
- [x] `TASK-260712-m5264f` — self-service-onboarding-capability-api
- [x] `TASK-260712-6kba80` — select-appcontainer-capture-bridge
- [x] `TASK-260712-dib11l` — build-packaged-windows-probe

These six tasks remain accepted unless a later change invalidates their frozen
evidence. If that happens, perform a delta review; do not silently reopen or
silently inherit the old verdict.

## 1. Close the landed foundation checkpoints

Exit gate: both onboarding clients are accepted, auth migration/rollback is
proven, the lifecycle implementation has a fresh review boundary, and a signed
Windows 10/11 hardware matrix establishes which downstream assumptions are real.

- [ ] `TASK-260712-2u1w16` — macos-keychain-onboarding-client (checkpoint: finish R5 rework, strict retry parsing and observable clipboard-cleanup failure; fresh security/migration review)
- [ ] `TASK-260712-47uve0` — windows-dpapi-onboarding-client (checkpoint: restart substantive review on the current frozen files; include native DPAPI/clipboard and installed-package migration evidence)
- [ ] `TASK-260712-38qsku` — auth-migration-rollback-verification
- [ ] `TASK-260712-2y74io` — handle-probe-lifecycle-cleanup (checkpoint: refreeze post-CI bytes, rerun adversarial R11 review and root audit)
- [ ] `TASK-260712-13rbnw` — package-signed-msix-probe
- [ ] `TASK-260712-1vtwkl` — run-win10-win11-evidence-matrix

## 2. P1 generic media ingest and storage

Story: `STORY-260712-ld674h` — P1 Generic media ingest and storage.

- [ ] `TASK-260712-z6h6wh` — media-schema-repositories
- [ ] `TASK-260712-1bnos4` — upload-session-api-auth
- [ ] `TASK-260712-2af2dp` — submitmedia-processing-pipeline
- [ ] `TASK-260712-1sae4q` — media-delete-retention-cleanup
- [ ] `TASK-260712-3mcof4` — media-download-target-acl
- [ ] `TASK-260712-12ojcb` — telegram-submitmedia-compat
- [ ] `TASK-260712-gj0cko` — media-acl-delete-retention
- [ ] `TASK-260712-3huupe` — media-ingest-acceptance-tests
- [ ] `TASK-260712-jolzhh` — media-ingest-docs-handoff

## 3. P1 transmission protocol and scheduler

Story: `STORY-260712-25lysg` — P1 Transmission protocol and scheduler.

- [ ] `TASK-260712-51y5k9` — transmission-contract-clarification
- [ ] `TASK-260712-1aprcb` — transmission-store-target-snapshots
- [ ] `TASK-260712-1g70av` — clip-transmission-wire-contract
- [ ] `TASK-260712-2qpp6w` — transmission-http-resolution
- [ ] `TASK-260712-26ip33` — macos-transmission-client-hooks
- [ ] `TASK-260712-2bbz13` — windows-transmission-client-hooks
- [ ] `TASK-260712-31vvjt` — overlay-controller-scheduler
- [ ] `TASK-260712-2qc27p` — transmission-regression-coverage
- [ ] `TASK-260712-2cdjq8` — transmission-rollout-handoff

## 4. P1 policy and moderation foundation

Story: `STORY-260712-1tgryz` — P1 Policy and moderation foundation.

- [ ] `TASK-260712-16zfvu` — confirm-legal-ops-inputs
- [ ] `TASK-260712-2kec2s` — moderation-control-plane
- [ ] `TASK-260712-g9ycx5` — verify-current-store-policy
- [ ] `TASK-260712-1epb3a` — privacy-ugc-policy-pack
- [ ] `TASK-260712-1x0lot` — publish-policy-support-pages
- [ ] `TASK-260712-3t9nr8` — moderation-runbook-mailbox

## 5. P1 cross-platform overlay and interrupt mixer

Story: `STORY-260712-fes2jj` — P1 Cross-platform overlay and interrupt mixer.

- [ ] `TASK-260712-1hqiek` — render-safe-clip-state-foundation
- [ ] `TASK-260712-1viwvi` — windows-overlay-duck-limiter
- [ ] `TASK-260712-2zbmq4` — macos-overlay-duck-limiter
- [ ] `TASK-260712-1g6lk8` — windows-interrupt-resume
- [ ] `TASK-260712-8mwyiv` — macos-interrupt-resume
- [ ] `TASK-260712-3d6cnn` — overlay-interrupt-regression-tests
- [ ] `TASK-260712-2hodti` — overlay-interrupt-live-evidence

## 6. P1 Telegram adapter, history and presence

Story: `STORY-260712-34kbkn` — P1 Telegram adapter, history and presence.

- [ ] `TASK-260712-3coble` — phase1-history-presence-contract
- [ ] `TASK-260712-1gx6mh` — shared-delivery-presentation-model
- [ ] `TASK-260712-3dmllz` — telegram-callback-audio-transport
- [ ] `TASK-260712-1c1ska` — presence-dnd-block-surface
- [ ] `TASK-260712-2hcq1g` — transmission-history-receipt-query
- [ ] `TASK-260712-21ers7` — telegram-inline-routing-compat
- [ ] `TASK-260712-3e4p0c` — history-replay-policy-actions
- [ ] `TASK-260712-3d0zgu` — telegram-parity-regression-tests
- [ ] `TASK-260712-1f9jtm` — telegram-parity-docs-handoff

## 7. P1 main UI, local self-test and capture

Story: `STORY-260712-2e36uz` — P1 Main UI, local self-test and capture.

- [ ] `TASK-260712-1c04pk` — macos-main-window-menubar-shell
- [ ] `TASK-260712-2lrpc0` — builtin-cue-temp-media-contract
- [ ] `TASK-260712-30abcm` — macos-microphone-capture-engine
- [ ] `TASK-260712-9i5se7` — windows-main-window-tray-shell
- [ ] `TASK-260712-2w4gyw` — windows-microphone-capture-engine
- [ ] `TASK-260712-3lg0ht` — macos-self-test-file-intake
- [ ] `TASK-260712-ut6akw` — macos-hotkey-menubar-recording
- [ ] `TASK-260712-25at8b` — windows-self-test-file-intake
- [ ] `TASK-260712-c7dmv8` — windows-hotkey-tray-recording
- [ ] `TASK-260712-1s6h6t` — macos-local-capture-self-test
- [ ] `TASK-260712-1p8ykc` — windows-local-capture-self-test
- [ ] `TASK-260712-3dqc3l` — macos-ui-data-integration
- [ ] `TASK-260712-2fe5bz` — windows-ui-data-integration
- [ ] `TASK-260712-e5mfqj` — cross-platform-ui-verification

## 8. P1 Store compliance and acceptance

Story: `STORY-260712-1i0doc` — P1 Store compliance and acceptance. This is the
hard stop before P2.

- [ ] `TASK-260712-1cdoxh` — acceptance-env-gate-repair
- [ ] `TASK-260712-pbfz37` — windows-report-block-delete
- [ ] `TASK-260712-34stvx` — macos-report-block-delete
- [ ] `TASK-260712-dlltnr` — telegram-moderation-parity
- [ ] `TASK-260712-e1ie4x` — platform-declarations-localized-copy
- [ ] `TASK-260712-176b74` — p1-independent-protocol-review
- [ ] `TASK-260712-1uz0za` — p1-independent-realtime-audio-review
- [ ] `TASK-260712-1xkn75` — p1-independent-migration-review
- [ ] `TASK-260712-wy05n6` — p1-independent-security-review
- [ ] `TASK-260712-2s4e9p` — store-listing-iarc-assets
- [ ] `TASK-260712-38lssj` — p1-root-integration-review
- [ ] `TASK-260712-1xik11` — a1-a8-evidence-store-submit

## 9. P2 Air rooms and approach migration

Story: `STORY-260712-3v14m9` — P2 Air rooms and approach migration. This story
is executed before the other canonical Phase 8 story because it is on the epic
critical path.

- [ ] `TASK-260712-17yizc` — air-lifecycle-policy-contract
- [ ] `TASK-260712-3n36ny` — air-schema-link-migration
- [ ] `TASK-260712-kr64r2` — air-runtime-session-resolution
- [ ] `TASK-260712-2vhf80` — air-control-plane-api
- [ ] `TASK-260712-25862f` — air-policy-enforcement
- [ ] `TASK-260712-2bjdlb` — approach-air-alias-compat
- [ ] `TASK-260712-2i3u7v` — macos-air-room-data-integration
- [ ] `TASK-260712-31zja2` — windows-air-room-data-integration
- [ ] `TASK-260712-2zdetx` — telegram-air-lifecycle-parity
- [ ] `TASK-260712-3nq0tq` — air-lifecycle-regression-rehearsal

## 10. P2 codec and streaming player spike

Story: `STORY-260712-3l1r1u` — P2 Codec and streaming player spike.

- [ ] `TASK-260712-14u0yk` — freeze-codec-spike-rubric-fixtures-harness
- [ ] `TASK-260712-dqdoqj` — prototype-stream-variants-range-cache-contract
- [ ] `TASK-260712-1vdlkw` — audit-codec-licenses-and-distribution-constraints
- [ ] `TASK-260712-1canzv` — probe-bundled-signed-decoder-path
- [ ] `TASK-260712-298tyq` — probe-media-foundation-appcontainer-path
- [ ] `TASK-260712-350u8d` — probe-macos-native-streaming-decoder
- [ ] `TASK-260712-3vkcki` — probe-pure-go-streaming-decoder-path
- [ ] `TASK-260712-ibuaxj` — run-comparative-streaming-evidence-matrix
- [ ] `TASK-260712-2eympi` — publish-codec-player-adr-and-handoff

## 11. P2 explicit targets, inbox and transport parity

Story: `STORY-260712-ob1tx2` — P2 Explicit targets, inbox and transport parity.

- [ ] `TASK-260712-2rlkp7` — target-inbox-contract-clarification
- [ ] `TASK-260712-1c34fe` — common-explicit-target-service
- [ ] `TASK-260712-2bk0vy` — target-inbox-store-acl
- [ ] `TASK-260712-2ctf3x` — versioned-content-policy-consent
- [ ] `TASK-260712-2j5fkr` — inbox-history-api-pagination
- [ ] `TASK-260712-2zoy4u` — rights-report-disable-enforcement
- [ ] `TASK-260712-2vipy3` — pulsar-inbox-history-ui
- [ ] `TASK-260712-2nto40` — macos-p2-targets-inbox-ui
- [ ] `TASK-260712-cuplon` — windows-p2-targets-inbox-ui
- [ ] `TASK-260712-1vklop` — targets-inbox-parity-regressions
- [ ] `TASK-260712-20cuna` — targets-inbox-rollout-handoff

## 12. P2 streamed user audio tracks

Story: `STORY-260712-2ori1t` — P2 Streamed user audio tracks.

- [ ] `TASK-260712-1n5fks` — stream-track-schema-variants
- [ ] `TASK-260712-31rkpe` — stream-track-wire-contract
- [ ] `TASK-260712-2ogntd` — stream-storage-egress-accounting
- [ ] `TASK-260712-285pag` — audio-track-variant-pipeline
- [ ] `TASK-260712-3lf8r0` — stream-range-serving-revocation
- [ ] `TASK-260712-2h6snp` — streamed-track-coordinator-flow
- [ ] `TASK-260712-17w78q` — windows-streamed-track-player
- [ ] `TASK-260712-1q2kwa` — stream-track-ui-model
- [ ] `TASK-260712-3aj8w2` — macos-streamed-track-player
- [ ] `TASK-260712-wt2n7m` — telegram-explicit-target-parity
- [ ] `TASK-260712-3lximx` — windows-stream-track-ui
- [ ] `TASK-260712-2psvhu` — macos-stream-track-ui
- [ ] `TASK-260712-1fpb9q` — streamed-track-regression-evidence
- [ ] `TASK-260712-2ubzyf` — streamed-track-rollout-handoff

## 13. P2 acceptance, capacity and rollout

Story: `STORY-260712-1qfbiw` — P2 Acceptance, capacity and rollout. The
promotion packet is the hard stop before P3.

- [ ] `TASK-260712-14rxuk` — phase2-gate-matrix-evidence-contract
- [ ] `TASK-260712-2g3fkt` — p2-independent-codec-supply-review
- [ ] `TASK-260712-28mn7w` — p2-independent-stream-performance-review
- [ ] `TASK-260712-2sicfs` — p2-independent-air-migration-review
- [ ] `TASK-260712-n11rg6` — p2-independent-target-security-review
- [ ] `TASK-260712-qi81vf` — phase2-observability-quota-views
- [ ] `TASK-260712-1kfnpu` — p2-root-integration-review
- [ ] `TASK-260712-21kz3b` — phase2-b2-b4-air-scale-acceptance
- [ ] `TASK-260712-2bdi4a` — phase2-b1-track-platform-matrix
- [ ] `TASK-260712-3qybi2` — phase2-rollout-migration-rollback
- [ ] `TASK-260712-3u5cdn` — phase2-b5-b7-rights-mixed-fleet
- [ ] `TASK-260712-2pnc5a` — phase2-beta-quota-calibration
- [ ] `TASK-260712-3a0cf9` — phase2-promotion-packet

## 14. P3 near-live push-to-talk

Story: `STORY-260712-sskhip` — P3 Near-live push-to-talk. This story is
executed before soundboard automation because it is on the epic critical path.

- [ ] `TASK-260712-9wivva` — store-safe-hold-input-spike
- [ ] `TASK-260712-lo7a68` — live-codec-transport-spike
- [ ] `TASK-260712-3qviqc` — live-ptt-wire-contract-codec-policy
- [ ] `TASK-260712-3vzbbl` — coordinator-live-ptt-session-runtime
- [ ] `TASK-260712-19w1qn` — macos-live-jitter-receiver
- [ ] `TASK-260712-26mnp1` — macos-live-capture-sender
- [ ] `TASK-260712-1ckdr7` — windows-live-jitter-receiver
- [ ] `TASK-260712-ezdhpf` — windows-live-capture-sender
- [ ] `TASK-260712-2kj9kj` — macos-live-ptt-node-integration
- [ ] `TASK-260712-2jbo5i` — windows-live-ptt-node-integration
- [ ] `TASK-260712-1rzqh9` — live-ptt-regression-evidence

## 15. P3 soundboard and safe automation

Story: `STORY-260712-326wd5` — P3 Soundboard and safe automation.

- [ ] `TASK-260712-3sj8ox` — automation-surface-safety-contract
- [ ] `TASK-260712-hb5xz2` — saved-cue-media-lifecycle
- [ ] `TASK-260712-3sv87k` — automation-schema-lineage-foundation
- [ ] `TASK-260712-1kk8bd` — cue-schedule-token-control-apis
- [ ] `TASK-260712-1eva0y` — automation-runtime-revoke-ratelimits
- [ ] `TASK-260712-11e4e3` — automation-history-audit-disable
- [ ] `TASK-260712-1yw7fo` — windows-soundboard-hotkeys-schedules
- [ ] `TASK-260712-288j4a` — macos-soundboard-hotkeys-schedules
- [ ] `TASK-260712-uht9e2` — telegram-soundboard-automation-parity
- [ ] `TASK-260712-89fzlc` — windows-automation-admin-ui
- [ ] `TASK-260712-1oodka` — macos-automation-admin-ui
- [ ] `TASK-260712-2f0gpu` — automation-safety-evidence-handoff

## 16. P3 end-to-end encrypted media

Story: `STORY-260712-1frfmi` — P3 End-to-end encrypted media. This story is
executed before capture quality because it is on the epic critical path.

- [ ] `TASK-260712-2e2ymn` — e2ee-threat-model-claims
- [ ] `TASK-260712-16xmy2` — protected-media-container-prep-spike
- [ ] `TASK-260712-3er89x` — group-crypto-library-spike
- [ ] `TASK-260712-2ys1ww` — e2ee-protocol-key-lifecycle-contract
- [ ] `TASK-260712-aniuyy` — e2ee-independent-design-review
- [ ] `TASK-260712-3w1cst` — encrypted-media-schema-epoch-foundation
- [ ] `TASK-260712-20j5tm` — coordinator-ciphertext-routing-rotation
- [ ] `TASK-260712-1yz5ca` — coordinator-opaque-media-router
- [ ] `TASK-260712-1x9ruo` — macos-e2ee-key-state
- [ ] `TASK-260712-25dzp4` — windows-e2ee-key-state
- [ ] `TASK-260712-2i0w6x` — report-evidence-moderation-export
- [ ] `TASK-260712-1rziyo` — recovery-device-transfer-history-grants
- [ ] `TASK-260712-2kcduo` — macos-protected-media-send
- [ ] `TASK-260712-tcwn44` — macos-protected-media-playback
- [ ] `TASK-260712-3980vy` — macos-e2ee-live-ptt
- [ ] `TASK-260712-28zhpl` — windows-protected-media-send
- [ ] `TASK-260712-1u57qz` — windows-protected-media-playback
- [ ] `TASK-260712-39vjzd` — windows-e2ee-live-ptt
- [ ] `TASK-260712-2nppt6` — macos-encrypted-media-client-path
- [ ] `TASK-260712-2q4jbu` — windows-encrypted-media-client-path
- [ ] `TASK-260712-1bcpda` — e2ee-c4-c6-evidence-review-pack

## 17. P3 capture quality and diagnostics

Story: `STORY-260712-3pt00e` — P3 Capture quality and diagnostics.

- [ ] `TASK-260712-1gmsvh` — freeze-capture-quality-contract
- [ ] `TASK-260712-265o0f` — probe-windows-voice-processing-path
- [ ] `TASK-260712-2gaswa` — probe-macos-voice-processing-path
- [ ] `TASK-260712-1pw1l1` — capture-diagnostics-capability-surface
- [ ] `TASK-260712-39czd2` — capture-quality-regression-harness
- [ ] `TASK-260712-2egweh` — macos-live-capture-effects
- [ ] `TASK-260712-wcdz08` — windows-live-capture-effects
- [ ] `TASK-260712-1getbv` — macos-capture-quality-ui
- [ ] `TASK-260712-39zh8g` — windows-capture-quality-ui
- [ ] `TASK-260712-1023d7` — capture-quality-integrated-regressions
- [ ] `TASK-260712-2e80pr` — c3-evidence-capability-matrix

## 18. P3 security acceptance and rollout

Story: `STORY-260712-2ft5wd` — P3 Security acceptance and rollout. No
capability is released merely because another capability passed its gate.

- [ ] `TASK-260712-3da0vz` — phase3-gate-matrix-evidence-contract
- [ ] `TASK-260712-2uo81g` — phase3-observability-health-evidence-views
- [ ] `TASK-260712-3g0axs` — phase3-root-line-review
- [ ] `TASK-260712-1ulshp` — phase3-external-security-review-closure
- [ ] `TASK-260712-3j4a06` — phase3-independent-realtime-review
- [ ] `TASK-260712-1x5jfo` — phase3-independent-automation-review
- [ ] `TASK-260712-7ng1vs` — phase3-independent-privacy-store-review
- [ ] `TASK-260712-flaiie` — phase3-c1-c3-live-platform-matrix
- [ ] `TASK-260712-yj668d` — phase3-c4-c6-reviewed-e2ee-acceptance
- [ ] `TASK-260712-1gyohk` — phase3-c7-automation-safety-acceptance
- [ ] `TASK-260712-30xwu2` — phase3-rollout-rollback-recovery-drills
- [ ] `TASK-260712-6mz9xg` — phase3-independent-migration-recovery-review
- [ ] `TASK-260712-1actom` — phase3-beta-soak-incident-review
- [ ] `TASK-260712-3b7bp4` — phase3-promotion-packet-disclosures
- [ ] `TASK-260712-2b5685` — phase3-root-final-release-audit

## Milestone gates

- P1 closes only after `TASK-260712-1xik11` has reproducible A1-A8 evidence and
  the actual Store submission state is captured.
- P2 starts only after P1 closes; P3 starts only after
  `TASK-260712-3a0cf9` and the required seven-day P2 beta gate close.
- Phase 3 release requires independent capability evidence for live PTT, E2EE,
  capture quality and automation, followed by the final root audit.
- Any code change after a frozen review/build hash triggers delta review and, if
  it affects a soak candidate, resets the corresponding elapsed-time gate.

## External inputs that can legitimately stop the sequence

- real supported Windows 10 and Windows 11 hardware and signing/Store identity;
- supported macOS hardware and representative speaker/headphone/Bluetooth/USB
  routes;
- real legal entity, support and moderation contacts plus public policy hosting;
- Partner Center authority, current Store declarations and real screenshots;
- qualified independent crypto, realtime, automation, privacy/Store and
  migration/recovery reviewers;
- real multi-home beta participants, two networks and uninterrupted seven-day
  P2/P3 soak windows.
