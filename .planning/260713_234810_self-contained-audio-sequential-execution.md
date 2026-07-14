# Sequential execution plan: Self-contained Pulsar Audio

- Date: 2026-07-13
- Engineering epic: `EPIC-260712-3agrc1` — Self-contained Pulsar Audio engineering
- Manual test epic: `EPIC-260714-th54l3` — Manual real-app hardware testing
- Baseline: `main` at merge commit `38ebd385e105eb2f6c7012c608cd1debfa3aad5e` (PR #9)
- Combined inventory: 205 original tasks; 14 accepted, 191 remain.
- Routed inventory: 186 engineering tasks (14 accepted, 172 remain) and 19
  deferred manual-test tasks (0 accepted, 19 remain).

## Execution status

- Started: 2026-07-14
- Mode: strict sequential inline execution; no task-board spawn workflow
- Current engineering task: `TASK-260712-1sae4q` — media-delete-retention-cleanup
- Current branch: `task/task-260712-1sae4q-media-delete-retention-cleanup`
- Accepted overall: 14 / 205 tasks (approximately 6.8%); 191 remain
- Engineering progress: 14 / 186 tasks (approximately 7.5%); 172 remain
- Manual-test progress: 0 / 19 tasks; all remain deferred
- State: the physical H00-H17 task and 18 later real-app, platform,
  production-shaped or beta acceptance tasks were moved to
  `EPIC-260714-th54l3`. Their evidence is not claimed passed and they no longer
  block best-effort coding, unit tests, deterministic integration tests, CI,
  packaging or engineering review. PR #10 landed this boundary and the Windows
  evidence harness on `main` at `06a06c099ed5b4f37f5e2dd3648772ffd041dfd9`.
  `TASK-260712-z6h6wh` landed through PR #11 at merge commit `31bbeb9`;
  `TASK-260712-1bnos4` landed through PR #12 at merge commit `050c979`;
  `TASK-260712-2af2dp` landed through PR #13 at merge commit `451e50b`; strict
  execution is now on `TASK-260712-1sae4q`.

Checkpoint 2026-07-14: the current task now has a strict H00-H17 collector,
privacy and package-provenance checks, immutable evidence references, cleanup
verification, and negative contract tests. Local Go vet/test, Windows amd64
cross-build, YAML and task-board validation are green. Hosted Windows CI run
`29295222623` also passed all four jobs; its inspected artifact and receipts are
recorded in the task resource. That remains a tooling regression gate and
cannot satisfy either physical OS row.

Root delta-review closed unsafe cleanup-output placement and the missing
rendered matrix. Commit `829bebb` passed all jobs in CI run `29295847330`; its
negative test proves rejection occurs before package mutation. The latest
artifact/receipt hashes are recorded on the board. A repeated access audit
still found no Windows boot volume, Windows Tailscale peer, SSH alias or
authorized self-hosted runner inventory.

The H00-H17 readiness details remain recorded in the task-board outcome
resource `TASK-260712-1vtwkl_hardware-readiness-audit.md`. That task is now a
manual-program backlog item, still at 0/36 physical rows. The complete manual
sequence is tracked in
`.planning/260714_045154_epic-260714-th54l3.md`.

Checkpoint 2026-07-14: `TASK-260712-z6h6wh` now has an additive five-table
generic media model, CAS-protected upload/media/storage transitions, a durable
publish/cleanup outbox, an explicit legacy WAV bridge and atomic media
revocation on orbit dissolution. Fresh/migrated DB tests, concurrent
idempotency/offset tests, injected publish/cleanup rollback, SQLite plaintext
artifact scans and an exact `06a06c0` predecessor round trip are green under
Go test/race/vet. Root review R1 tightened MIME, codec, loudness JSON and scoped
token validation; the remediated commit `ecc034b` passed all four jobs in
hosted CI run `29298686287`, including node-core on `macos-15`, the exact
predecessor coordinator gate and signed Windows packaging. The final tracking
commit passed all jobs again in run `29298874048`; PR #11 landed at
`31bbeb9257b2555c86858c4087521466b58d673a`, and strict execution advanced to
`TASK-260712-1bnos4`.

Checkpoint 2026-07-14: `TASK-260712-1bnos4` now exposes control-authenticated,
idempotent upload creation and scoped append-only PUTs with HMAC-remintable
capabilities, atomic start/concurrency/daily/hard-byte quotas, CAS offsets,
actual-length enforcement and stable non-disclosing failures. Staged fsync,
crash-tail truncation, zero-byte finalize recovery, scheduled expiry and a
durably acknowledged temp cleanup survive a real store reopen. Full local Go
test/race/vet, 20x focused concurrency stress, pulsar-win vet/test/cross-build,
task-board validation and the exact `31bbeb9` predecessor round trip are green.
All four hosted jobs passed on commit `b1b7576` in CI run `29300399021`,
including node-core on macOS and the signed Windows package probe. Root delta
review closed orphan staging cleanup, private-mode enforcement and concurrent
quota reservation. The final tracking bytes passed all four jobs again in CI
run `29300559446`; PR #12 landed at
`050c9792e328730e33bb65cf03fcda8e3d690061`, and strict execution advanced to
`TASK-260712-2af2dp`.

Checkpoint 2026-07-14: `TASK-260712-2af2dp` implementation now has a shared
transport-neutral `SubmitMedia` service, app-upload finalization wiring,
signature and bounded ffprobe validation, fixed/network-disabled ffmpeg,
Linux kernel CPU/memory/fd/file caps, canonical PCM WAV metadata and a durable
hard-link plus CAS publication boundary. Store-backed tests cover ready/failed
state, cleanup, idempotent and concurrent retry, same-orbit-only physical
dedupe and crash recovery after the atomic link; HTTP tests cover interrupted
`finalizing` recovery and non-disclosing errors. The exact immediate
predecessor `050c979` rollback test and local full Go test/race/vet gates are
green. Root delta review then closed aggregate worker admission, staging and
recovered-file fsync, MP3/FLAC framing, chapter stripping and app session/path
binding. Hosted CI run `29302835228` passed all four jobs on final code commit
`097bcf8`, including the live Linux six-format, HTTP and rlimit paths, macOS
Swift, portable Windows and signed-MSIX regressions. The final tracking commit
passed all four jobs again in CI run `29302971194`; PR #13 landed at
`451e50bb1375b7db85b6e909c0ae4ef256efd2cc`, and strict execution advanced to
`TASK-260712-1sae4q`. No manual real-app or hardware claim is inherited.

Checkpoint 2026-07-14: `TASK-260712-1sae4q` is in review on branch
`task/task-260712-1sae4q-media-delete-retention-cleanup`. It implements atomic
owner-orbit control deletion, immediate read revocation, the frozen
`media_lifecycle_v1` cancellation outbox, seven-day clip expiry, crash-safe
canonical and temporary cleanup, 90-day content-free audit pruning, health
metrics and the backup/privacy handoff. Coordinator vet, full tests, full race,
the exact `451e50b` predecessor round-trip, and portable Windows vet/test/build
are green. The production cancellation sink remains deliberately pending for
the later transmission tasks; local Swift lacks the `Testing` module and awaits
hosted macOS CI. No manual real-app or hardware result is claimed.

## Operating contract

This is a standalone execution plan. Task-board IDs are stable links to the
existing task briefs, acceptance criteria and evidence. Status, assignment and
notes are mirrored to task-board for tracking, but implementation and review
are performed inline without the task-board spawn workflow.

Execute exactly one unchecked engineering task at a time, from top to bottom.
Rows marked `↪ manual` are routing records, not executable steps in this plan.
Do not begin the next engineering task until the current task's implementation,
automatable tests, available evidence and required engineering review are
accepted and landed on `main`. Within a task:

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

If a task's remaining acceptance is hands-on real-app, physical-hardware,
production-shaped rollout or elapsed beta work, route that acceptance to
`EPIC-260714-th54l3` and continue engineering from documented assumptions. Do
not count a mock, developer-mode path, fabricated input or unreviewed build as
manual evidence. Missing legal/support data, public hosting, external reviewers
or mutation authority remains an honest engineering blocker unless the task can
produce a useful non-authoritative draft or handoff without it.

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
proven, and the lifecycle, signed-package and evidence harness engineering is
accepted. The physical Windows 10/11 matrix is deferred without being called
passed.

- [x] `TASK-260712-2u1w16` — macos-keychain-onboarding-client (R5 accepted 2026-07-14; focused 75/75, full 125/125, recovery/clipboard stress 100/100, release/format/privacy gates green)
- [x] `TASK-260712-47uve0` — windows-dpapi-onboarding-client (R4 code checkpoint accepted 2026-07-14 after same-executor cold R3 remediation; focused x50, full/race, privacy, amd64/arm64 cross-build gates green; native DPAPI/NTFS/HWND, installed-MSIX and Windows 10/11 claims remain explicit downstream gates)
- [x] `TASK-260712-38qsku` — auth-migration-rollback-verification (accepted 2026-07-14; exact pinned predecessor Store/config gates, callable fail-closed projection, physical SQLite secret scan, coordinator/macOS/Windows full matrices and operator runbook green; no live production or native Windows claim)
- [x] `TASK-260712-2y74io` — handle-probe-lifecycle-cleanup (accepted by frozen R13 adversarial review and root audit; signed Windows evidence remains in downstream packaging/hardware tasks)
- [x] `TASK-260712-13rbnw` — package-signed-msix-probe (accepted 2026-07-14; current Partner Center identity/PFN/AUMID frozen, non-exportable local signing and Store routes documented, signed MSIX build/registration/digest receipt green in CI; real Win10/11 hardware remains in the next task)
- ↪ manual `TASK-260712-1vtwkl` — run-win10-win11-evidence-matrix →
  `EPIC-260714-th54l3` / `STORY-260714-36vmp0`

## 2. P1 generic media ingest and storage

Story: `STORY-260712-ld674h` — P1 Generic media ingest and storage.

- [x] `TASK-260712-z6h6wh` — media-schema-repositories (accepted and landed:
  additive schema/CAS repositories, exact predecessor rollback, full local
  race plus hosted CI runs `29298686287` / `29298874048` green; PR #11,
  merge `31bbeb9`)
- [x] `TASK-260712-1bnos4` — upload-session-api-auth (accepted and landed:
  authenticated resumable HTTP, atomic quotas, crash-safe temp lifecycle,
  exact immediate-predecessor rollback, full local race plus hosted CI runs
  `29300399021` / `29300559446` green; PR #12, merge `050c979`)
- [x] `TASK-260712-2af2dp` — submitmedia-processing-pipeline (accepted and
  landed: shared constrained processing, durable atomic publication,
  retry/dedupe/failure lifecycle, exact predecessor rollback, hosted CI runs
  `29302835228` / `29302971194` green; PR #13, merge `451e50b`)
- [ ] `TASK-260712-1sae4q` — media-delete-retention-cleanup (implementation and
  local review evidence complete; hosted CI and merge pending)
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
- ↪ manual `TASK-260712-2hodti` — overlay-interrupt-live-evidence →
  `EPIC-260714-th54l3`

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
- ↪ manual `TASK-260712-e5mfqj` — cross-platform-ui-verification →
  `EPIC-260714-th54l3`

## 8. P1 Store compliance and engineering readiness

Story: `STORY-260712-1i0doc` — P1 Store compliance and engineering readiness.
This is the engineering stop before P2; it cannot assert manual or Store
acceptance.

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
- [ ] `TASK-260712-1xik11` — p1-engineering-readiness-handoff

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
- ↪ manual `TASK-260712-1fpb9q` — streamed-track-regression-evidence →
  `EPIC-260714-th54l3`
- [ ] `TASK-260712-2ubzyf` — streamed-track-rollout-handoff

## 13. P2 engineering integration and rollout readiness

Story: `STORY-260712-1qfbiw` — P2 engineering integration and rollout
readiness. The engineering handoff packet is the stop before P3 development;
manual promotion remains separate.

- [ ] `TASK-260712-14rxuk` — phase2-gate-matrix-evidence-contract
- [ ] `TASK-260712-2g3fkt` — p2-independent-codec-supply-review
- [ ] `TASK-260712-28mn7w` — p2-independent-stream-performance-review
- [ ] `TASK-260712-2sicfs` — p2-independent-air-migration-review
- [ ] `TASK-260712-n11rg6` — p2-independent-target-security-review
- [ ] `TASK-260712-qi81vf` — phase2-observability-quota-views
- [ ] `TASK-260712-1kfnpu` — p2-root-integration-review
- ↪ manual `TASK-260712-21kz3b` — phase2-b2-b4-air-scale-acceptance
- ↪ manual `TASK-260712-2bdi4a` — phase2-b1-track-platform-matrix
- ↪ manual `TASK-260712-3qybi2` — phase2-rollout-migration-rollback
- ↪ manual `TASK-260712-3u5cdn` — phase2-b5-b7-rights-mixed-fleet
- ↪ manual `TASK-260712-2pnc5a` — phase2-beta-quota-calibration
- [ ] `TASK-260712-3a0cf9` — phase2-engineering-handoff-packet

## 14. P3 near-live push-to-talk

Story: `STORY-260712-sskhip` — P3 Near-live push-to-talk. This story is
executed before soundboard automation because it is on the epic critical path.

- ↪ manual `TASK-260712-9wivva` — store-safe-hold-input-spike →
  `EPIC-260714-th54l3`
- [ ] `TASK-260712-lo7a68` — live-codec-transport-spike
- [ ] `TASK-260712-3qviqc` — live-ptt-wire-contract-codec-policy
- [ ] `TASK-260712-3vzbbl` — coordinator-live-ptt-session-runtime
- [ ] `TASK-260712-19w1qn` — macos-live-jitter-receiver
- [ ] `TASK-260712-26mnp1` — macos-live-capture-sender
- [ ] `TASK-260712-1ckdr7` — windows-live-jitter-receiver
- [ ] `TASK-260712-ezdhpf` — windows-live-capture-sender
- [ ] `TASK-260712-2kj9kj` — macos-live-ptt-node-integration
- [ ] `TASK-260712-2jbo5i` — windows-live-ptt-node-integration
- ↪ manual `TASK-260712-1rzqh9` — live-ptt-regression-evidence →
  `EPIC-260714-th54l3`

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
- ↪ manual `TASK-260712-265o0f` — probe-windows-voice-processing-path
- ↪ manual `TASK-260712-2gaswa` — probe-macos-voice-processing-path
- [ ] `TASK-260712-1pw1l1` — capture-diagnostics-capability-surface
- [ ] `TASK-260712-39czd2` — capture-quality-regression-harness
- [ ] `TASK-260712-2egweh` — macos-live-capture-effects
- [ ] `TASK-260712-wcdz08` — windows-live-capture-effects
- [ ] `TASK-260712-1getbv` — macos-capture-quality-ui
- [ ] `TASK-260712-39zh8g` — windows-capture-quality-ui
- [ ] `TASK-260712-1023d7` — capture-quality-integrated-regressions
- ↪ manual `TASK-260712-2e80pr` — c3-evidence-capability-matrix

## 18. P3 security and engineering completion

Story: `STORY-260712-2ft5wd` — P3 Security acceptance and rollout. No
capability is released merely because another capability passed its gate.

- [ ] `TASK-260712-3da0vz` — phase3-gate-matrix-evidence-contract
- [ ] `TASK-260712-2uo81g` — phase3-observability-health-evidence-views
- [ ] `TASK-260712-3g0axs` — phase3-root-line-review
- [ ] `TASK-260712-1ulshp` — phase3-external-security-review-closure
- [ ] `TASK-260712-3j4a06` — phase3-independent-realtime-review
- [ ] `TASK-260712-1x5jfo` — phase3-independent-automation-review
- [ ] `TASK-260712-7ng1vs` — phase3-independent-privacy-store-review
- ↪ manual `TASK-260712-flaiie` — phase3-c1-c3-live-platform-matrix
- ↪ manual `TASK-260712-yj668d` — phase3-c4-c6-reviewed-e2ee-acceptance
- ↪ manual `TASK-260712-1gyohk` — phase3-c7-automation-safety-acceptance
- ↪ manual `TASK-260712-30xwu2` — phase3-rollout-rollback-recovery-drills
- [ ] `TASK-260712-6mz9xg` — phase3-independent-migration-recovery-review
- ↪ manual `TASK-260712-1actom` — phase3-beta-soak-incident-review
- [ ] `TASK-260712-3b7bp4` — phase3-engineering-handoff-disclosures
- [ ] `TASK-260712-2b5685` — phase3-root-engineering-completion-audit

## Milestone gates

- P1 engineering closes after `TASK-260712-1xik11` freezes the reviewed build,
  automated evidence and exact manual-test handoff. It does not submit or claim
  acceptance in Partner Center.
- P2 engineering starts after P1 engineering closes; P3 engineering starts
  after `TASK-260712-3a0cf9` publishes the reviewed engineering packet. The
  seven-day P2 beta remains pending in the manual epic.
- `TASK-260712-2b5685` closes engineering only. Store or production promotion
  additionally requires every applicable task in `EPIC-260714-th54l3`.
- Any code change after a frozen review/build hash triggers delta review and, if
  it affects a soak candidate, resets the corresponding elapsed-time gate.

## External inputs routed outside the engineering sequence

- real supported Windows 10 and Windows 11 hardware and physical audio devices;
- supported macOS hardware and representative speaker/headphone/Bluetooth/USB
  routes;
- real multi-home beta participants, two networks and uninterrupted seven-day
  P2/P3 soak windows.

These inputs hold `EPIC-260714-th54l3`, not this engineering sequence. The
following non-test inputs may still block the engineering task that consumes
them:

- real legal entity, support and moderation contacts plus public policy hosting;
- Partner Center authority, current Store declarations and real screenshots;
- qualified independent crypto, realtime, automation, privacy/Store and
  migration/recovery reviewers;
