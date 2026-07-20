# Sequential execution plan: Self-contained Pulsar Audio

- Date: 2026-07-13
- Engineering epic: `EPIC-260712-3agrc1` — Self-contained Pulsar Audio engineering
- Manual test epic: `EPIC-260714-th54l3` — Manual real-app hardware testing
- Baseline: `main` at merge commit `38ebd385e105eb2f6c7012c608cd1debfa3aad5e` (PR #9)
- Combined inventory: 205 original tasks; 183 accepted, 22 remain.
- Routed inventory: 186 engineering tasks (183 accepted, 3 remain) and 19
  deferred manual-test tasks (0 accepted, 19 remain).

## Execution status

- Started: 2026-07-14
- Mode: strict sequential inline engineering execution; task-board tracked
  spawn is used only for explicitly owner-authorized independent reviewers.
- Current engineering task: `TASK-260712-2q4jbu` —
  windows-encrypted-media-client-path in deferred epic `EPIC-260716-3qsztl`.
- Current original-plan frontier: `TASK-260712-2q4jbu` —
  windows-encrypted-media-client-path in deferred epic `EPIC-260716-3qsztl`;
  it starts only after the accepted macOS client-path branch is merged.
  The preceding p1-independent-security-review
  (`TASK-260712-wy05n6`) was independently approved on 2026-07-19 by Claude
  Fable 5, spawned through task-board as run `RUN-260719-ca4eaf` on owner-gate
  task `TASK-260715-10ksxz`. The reviewer verified exact main head `1b9207e`
  (a later exact head than the frozen audit merge `dab3999`), confirmed the
  reviewer implemented none of the reviewed paths, re-reviewed the three closed
  HIGH findings (P1-SEC-001/002/003, with P1-SEC-002 strengthened post-audit)
  and all eleven trust boundaries with no secret/audio/tenant leak, accepted
  every medium disposition (M01/M02/M03) and dispositioned migration MED-1 as
  non-blocking with tracked follow-up `BUG-260719-1rsd49`, and confirmed
  coordinator/pulsar-win race suites plus govulncheck clean locally and hosted
  run `29692957096` passing 4/4 jobs. The original security review is accepted
  (`TASK-260712-wy05n6` done, checklist item 1 checked). Remaining Store/IARC
  completion is the strict hold and requires its own evidence and verdict; this
  signoff makes no real-app or hardware claim. Tracking PR #279 merged at
  `aee65ba`; its post-review hosted run `29693870942` passed 4/4 jobs.
- Security hardening follow-up: `BUG-260719-1rsd49` is accepted and done.
  Producer commit `da6b4cb` carries SQLite pragmas onto every replacement
  connection, requires persisted Store authorization or a descriptor-pinning
  lease for media target readers, and documents `withControl` as preflight-only.
  Focused, full and full race suites passed (Store race 441.840 s). Independent
  Claude Fable 5 max runs `RUN-260719-f2757d` and `RUN-260719-c395cf` inspected
  all three boundaries with no HIGH issue; terminal run `RUN-260719-0e576a`
  reconfirmed byte-identical production code, ran fresh focused checks and
  recorded APPROVE in `BUG-260719-1rsd49_reviewer-verdict.md`. This closed the
  last open item in `EPIC-260712-3agrc1` and releases the deferred E2EE line.
- Migration gate: the owner-authorized independent migration review ran on
  2026-07-19 (Claude Fable 5, run `RUN-260719-d82ed0`) at exact main head
  `06ce330` containing audit merge `d7e0065`. The audit packet was validated
  across all six layers; `P1-MIG-001`/`P1-MIG-002` were confirmed closed with
  failure, partial, ten-run concurrent, 14/14 exact-predecessor and
  full-module race reruns green locally plus hosted run `29690180035` (4/4
  jobs). Approval is withheld on one new post-audit HIGH: `P1-MIG-003`, a
  dissolution-reconciler ordering regression — `reconcileOrphanedMediaItems`
  (inside `initMediaIngestSchema`, `store.go:146`) now revokes through
  `saved_cues`/`transmission_inbox_items` (`media_ingest.go:1727/1733`)
  before `store.go:158/170` create them, so a pre-inbox/pre-cue-generation
  database holding an active predecessor-dissolution orphan fails roll-forward
  startup deterministically. The producer correction now makes media-ingest
  initialization DDL-only and runs all media reconciliation after transmission
  inbox and saved-cue schema installation. The new generation-skip fixture
  proves both absent tables are recreated before orphan revocation, exactly one
  cleanup receipt is recorded, and restart is idempotent. Focused migration
  race tests, the full coordinator suite, full coordinator race suite and the
  complete `previoushead`-tagged store race suite pass. Final independent
  run `RUN-260719-c83d59` reproduced the pre-fix failure, verified the
  correction and fixture at exact PR head `aafcfc2`, independently reran the
  full coordinator race suite (store 455.8 s), passed predecessor 13/13 and
  consumed hosted CI run `29691922727` (4/4). It recorded APPROVE and closed
  P1-MIG-003. Final evidence:
  `TASK-260715-unbb7c_final-approval-verdict.md`.
- E2EE independent design gate: Claude Fable 5 max run
  `RUN-260719-1bbaa7` independently approved the exact `7e6c8be` packet for
  `TASK-260712-aniuyy`. It reproduced all twelve frozen packet hashes, ran the
  coordinator, Windows, macOS and Python acceptance suites, confirmed the
  capability remains dormant with no runtime callsite or crypto dependency,
  and found no open Critical/High design issue. IDR-001 through IDR-003 are
  tracked Low follow-ups for canonical multi-fault precedence, sequence/reset
  vectors and strict envelope decoding; IDR-004 and IDR-005 are informational.
  The verdict makes no signed-app, real-device or hardware claim and requires
  delta review after protocol-affecting changes. Production gates EPC-001,
  EPC-002, EPC-004 and EPC-005 remain open by design.
- E2EE schema/epoch foundation: producer commit `b11377e` adds eleven additive
  public-state/ciphertext-only tables, exact conditional commit, replay,
  finalize/revoke and grant transitions, immutable payload/audit triggers,
  migration/rollback fixtures and a physically locked-off feature row. The
  same delta closes IDR-001 through IDR-003 with canonical multi-fault
  precedence, shared sequence/generation vectors on coordinator/Windows/macOS
  and strict commit/proposal/welcome/key-package/history-grant decoders.
  Producer full/race/platform/acceptance suites passed (Store race 475.523 s).
  Independent Claude Fable 5 max run `RUN-260719-b1df39` reproduced all
  thirteen hashes, all suites (Store race 480.450 s), and a separate 10/10
  adversarial SQLite constraint probe, then recorded APPROVE with zero
  Critical/High/Medium findings. Low L1 tracks a few fail-closed multi-fault
  precedence corners; I1-I4 track sentinel-test strength, direct fork-freeze
  coverage, rejected-replay auditing and evidence-ref digest binding. The
  verdict does not accept production E2EE; later protocol changes require
  another delta review.
- E2EE coordinator routing/rotation foundation: exact producer commit
  `e97717bfad6348279430012ecf0ce3de404eae0d` adds four additive production-dark
  tables for protocol-actor bindings, exact member lineage, rotation
  requirements and durable per-device delivery. It serializes only
  client-produced signed proposals/commits through an injected verifier,
  reconciles join/leave/rejoin/role/revoke/disable changes fail closed, gates
  protected writes against current membership, and provides restart-safe
  collision-safe delivery/ack without selecting crypto or exposing runtime
  capability. Producer full test/vet, acceptance 207/207 and race suites passed
  (Store 502.193 s). Independent Claude Fable 5 max run
  `RUN-260719-47433f` reproduced all 12 pins, full/race/acceptance evidence
  (Store race 525.144 s), and APPROVED the exact SHA with zero open
  Critical/High/Medium findings. Non-blocking L1 records multi-cause
  `reason_code` audit fidelity; I1 requires downstream key-state work to pin
  the member-with-only-revoked-devices semantic. Production EPC gates and
  external security acceptance remain open. PR #285 passed hosted CI run
  `29702340139` (4/4) and merged to `main` as `32fee4ac`.
- E2EE opaque media router: exact producer commit
  `e4488ed2c0335e57910d704cf4bb4119593bbfdd` adds five additive tables,
  ciphertext-only manifest/chunk/range/delete routing, exact frozen recipient
  lineage, bounded upload/egress quotas and a distinct bounded `BE` opaque-live
  envelope with persisted public replay/rate/receipt state but no frame
  payload. Runtime HTTP/WS wiring, capability advertisement, production crypto
  selection and hardware/app claims remain absent. Producer full test/vet,
  acceptance 212/212, focused race and full race passed (Store full race
  594.955 s). Claude Fable 5 max completion run `RUN-260719-91776a`
  independently re-verified exact SHA, 14/14 pins, full test/vet, focused race,
  212/212 acceptance and previous-head rollback, then APPROVED with zero
  Critical/High/Medium findings. The broad race is honestly producer-only
  evidence and a non-blocking independent follow-up before activation.
- macOS E2EE key state: exact producer commit
  `498957eab686a4e6aad0f653813ccfe3d1d3efa6` separates device
  metadata, signing, agreement, group, grant and bounded content-cache state
  into dedicated device-only non-synchronizing Keychain slots with an
  independent witness per slot. Exact predecessor epochs, persist/readback
  before ack, partial-install/clone/replay/fork failure, ambiguous-success
  generation consumption, expiry/deletion, redacted leases and EPC-005 target
  semantics are covered. Focused Swift passed 10/10, full NodeCore passed
  318/318, acceptance passed 217/217 and swift-format is clean. Runtime
  capability, production crypto selection, physical Keychain, signed package,
  backup/restore and memory-forensics claims remain absent. Independent Claude
  Fable 5 max run `RUN-260719-20ab4a` reproduced all 9 hashes, focused 10/10,
  full Swift 318/318, full automated 16/16 and acceptance 217/217, then
  APPROVED WITH NON-BLOCKING FOLLOW-UPS with zero Critical/High. Medium M1
  records the lack of cross-process Keychain CAS and is now an explicit DoD on
  macOS send/playback/live/client-path integration: runtime must enforce one
  owning process or add cross-process serialization before wiring. Recovery
  inherits partial-install reset/re-enrollment and expired-grant cleanup. PR
  #287 then passed hosted CI run `29705960146` (4/4) and merged to `main` as
  `5f1756d57df16a476b2df353f60656d24b02f752`.
- Windows E2EE key state: exact final producer commit
  `c7c9b0206f61aa98920e8a21db55265fc9543b96` adds distinct
  current-user-DPAPI state/witness files for device metadata, signing,
  agreement, groups, grants and the bounded content-key cache. A
  repository-wide in-process plus Win32 share-none lock covers validation,
  write-through replace/readback and acknowledgment, closing cross-process
  double-reservation inside this foundation. Shared predecessor, crash,
  replay, clone, expiry, delete, target and lock vectors pass 10/10 and 20×
  under race; full test/vet/race, Windows amd64/arm64 vet/test-compile and
  acceptance 222/222 pass. A detached clean-worktree full harness also passed
  all 16/16 stages on the exact final SHA. Runtime wiring, production crypto, native DPAPI,
  signed MSIX, NTFS and profile backup/restore claims remain absent. Acceptance
  was granted by owner-authorized Claude Fable 5 max run
  `RUN-260719-c050cd`: APPROVE WITH NON-BLOCKING FOLLOW-UPS, zero
  Critical/High/Medium code findings. Its one Low finding on early decode-error
  secret cleanup at the first producer SHA was fixed by `c7c9b02`; the reviewer
  independently re-ran the full battery and 14/14 packet hashes at that final
  SHA. Manual evidence remains in `EPIC-260714-th54l3`.
- E2EE report evidence moderation export: exact producer commit
  `66a34edcbdf8c60fe5827041f0809930c46cfc69` adds a production-dark
  metadata-only report boundary and a separate explicit-consent transition
  for a bounded moderation-at-rest evidence reference. New exports bind the
  exact report, protected object, reporter actor/device, manifest, epoch,
  generation and revision, then re-authorize the current recipient; revoked
  access fails closed. List/Evidence/Decide operator capabilities, immutable
  content-free create/read/delete/expiry/decision audit, 30-day retention,
  statement scrub, crash rollback and idempotent restart are covered. E2EE
  delete reuses the canonical opaque chunk purge; actor/orbit actions reuse
  canonical disable/cancellation paths. Producer focused race, full Go/vet,
  227/227 acceptance and clean coordinator harness 7/7 passed. Independent
  Claude Fable 5 max run `RUN-260720-65a670` reproduced the exact SHA and full
  battery and ACCEPTED with no Critical/High/Medium finding. Six Low/Info notes
  are non-blocking and recorded in
  `TASK-260712-2i0w6x_independent-exact-sha-review.md`. No HTTP route, storage
  adapter, capability advertisement, coordinator decrypt, plaintext evidence,
  real-app, provider-delete or traffic-capture claim was added; manual scope
  remains in `EPIC-260714-th54l3` and production EPC gates remain open.
  Hosted CI run `29709135019` passed all four jobs; PR #289 merged to `main`
  as `f9fd2ec965e9b8b3396a10339541ae1327dd6a90`.
- E2EE recovery, device transfer and history grants: exact producer commit
  `94e506629c46473bc890575539750b1a993bbc50` adds production-dark current-epoch
  opaque transfer packages bound to the exact clean group and full issuer and
  recipient lineage, explicit one-time or time-bound named-object history
  grants, atomic expiry/revoke/audit and lost-device rotation. macOS and Windows
  add explicit fail-closed identity reset plus caller-enumerated bounded expired
  grant cleanup without relabeling old installation-bound state. Producer full
  automated harness passed 16/16; focused coordinator and Windows race suites
  and macOS 11/11 passed. Independent Claude Fable 5 max run
  `RUN-260720-6193e1` re-ran the complete 16/16 harness at the exact detached
  SHA and ACCEPTED with no Critical/High/Medium finding. One fail-closed error
  classification Low and two Info notes are recorded in
  `TASK-260712-1rziyo_review-verdict.md`. No runtime, production crypto,
  signed-package, real-device or recoverable-history claim is made; those
  remain manual/deferred in `EPIC-260714-th54l3`.
- macOS protected-media send foundation: exact producer commit
  `30d23def4350aab22a19824c1e0cbcfad1a5f8da` adds a dormant actor-owned
  prepare/stage/chunk/finalize pipeline behind an unselected provider seam.
  Rights and exact targets are admitted before the crash-safe key-state
  generation reservation; unsupported recipients cannot downgrade; exact
  ciphertext resumes without reseal or generation/nonce reuse; app-owned and
  user-owned plaintext have distinct bounded cleanup policies. A
  non-releasable per-repository send-owner claim closes the earlier
  process-local composition concern while explicitly not claiming
  cross-process serialization. Producer focused 12/12, full macOS 331/331,
  acceptance 190/190 and automated 16/16 passed. Independent Claude Fable 5
  max run `RUN-260720-cc3c8d` reproduced the exact-SHA evidence and all eight
  packet hashes, then ACCEPTED with zero Critical/High/Medium findings. Four
  Low and three Info runtime-integration follow-ups are recorded in
  `TASK-260712-2kcduo_review-verdict.md`; no production provider, runtime,
  signed-app, real-crypto, codec, hardware or memory-hygiene claim is made.
- macOS protected-media playback foundation: exact accepted rework commit
  `8c2676206f3fdb44ed54b9ad6f3dc1c5af5728af` adds a production-dark
  manifest/envelope and per-record authentication boundary, exact
  object/recipient/group/epoch/generation/target binding, live expiry/group/
  history-grant revalidation and ciphertext-only incremental range cache. The
  existing bounded player receives bytes only after provider authentication
  and retains the prepared lifetime owner. The initial exact-SHA review found
  one Medium multi-cache index race; the rework serializes shared-root index
  mutations, read-merges immutable entries, preserves tombstones as a
  monotonic union and uses unique temp names. Its regression reproduces the
  exact stale A-hit/B-revoke/restart failure. Rotation/expiry now invalidate
  without permanently blocking legitimate history re-grants. Producer focused
  9/9, player 6/6, full Swift 340/340, acceptance discovery 195/195 and exact
  automated harness 16/16 passed. Claude Fable 5 max run
  `RUN-260720-cf2797` ACCEPTED the exact rework SHA with no open
  Critical/High/Medium finding after independently reproducing the full
  battery and all packet hashes. One transport-expiry classification Low and
  four runtime integration Info notes are recorded in
  `TASK-260712-tcwn44_review-verdict-8c26762.md`; production provider/runtime,
  signed-app, real-crypto/codec, cross-process ownership and hardware/memory
  claims remain deferred. Hosted CI run `29713219537` passed all four jobs;
  PR #292 merged to `main` as `2aed6272dc153e584bd1371af93490285ffadaae`.
- macOS E2EE live-PTT foundation: exact accepted rework commit
  `c9faa7ef4a5cc089ebfb83bdce11fadfcfe669b8` mirrors the accepted coordinator
  `BE` opaque wire while keeping production crypto, runtime composition and
  capability advertisement dark. The sender reserves a witnessed `live_ptt`
  generation, seals off the capture callback and caches exact retry bytes; the
  receiver authenticates before jitter admission and fails closed on replay,
  nonce reuse, tamper, stale epoch, changed commit, target drift and removed
  membership. The first independent review correctly rejected device-local
  Keychain record revision in cross-device AAD. Rework binds shared witnessed
  epoch plus `commitDigest`, retains local revision only for setup/CAS, adds a
  two-installation skewed-revision round-trip, distinguishes malformed provider
  output and checks the 15,000-frame bound before seal. Producer and independent
  evidence passed strict formatting, focused 10/10, full Swift 350/350,
  acceptance 200/200 and automated 16/16. Claude Fable 5 max re-review run
  `RUN-260720-8f681f` ACCEPTED with zero open Critical/High/Medium; details are
  in `TASK-260712-3980vy_review-verdict-v2.md`. Real C1-C2, traffic capture,
  signed package, memory/crash, cross-process contention, macOS-Windows interop
  and production-provider evidence remain manual/deferred in
  `EPIC-260714-th54l3`; EPC-001/002/004/005 remain open. Hosted CI run
  `29715975166` passed all four jobs; PR #293 merged to `main` as
  `94d5de0fc36a0aae29f9f4026214c0a6324edf38`.
- Windows protected-media send foundation: exact accepted rework
  `b2a4af69530545ede4b82f31a451c556ef7c536f` provides the production-dark
  clip/track/saved-cue send boundary with witnessed cross-process-serialized
  generations, bounded plaintext ownership, exact provider context, strict
  ciphertext-only resumable state, idempotent stage/chunk/finalize/delete and
  durable published-revision checkpoints. The first independent review run
  `RUN-260720-6ead84` exposed two lifecycle repros: already-missing owned
  plaintext wedged cleanup/recovery, and a crash-created state-less final
  directory permanently blocked its draft ID. Rework now prepares under a
  private `.prepare-*` directory and atomically renames only complete state,
  rejects an orphan before reserving a generation, boundedly recovers legacy
  orphans, and makes missing-owned-plaintext cleanup convergent while foreign,
  directory and escaping-symlink paths remain fail-closed. Claude Fable 5 max
  run `RUN-260720-1e8fa2` re-ran both repros, audited the full boundary and
  ACCEPTED with zero open Critical/High/Medium. Focused 27/27 plus race,
  Windows key-state race, full Go plus race, vet, amd64/arm64 blind compiles,
  acceptance 205/205 and automated 16/16 passed; final manifest is
  `.temp/acceptance/20260720T050800Z/manifest.json`. Low L1 records possible
  stale ciphertext-only `.prepare-*` accumulation after repeated crashes;
  L2/L3 are informational. Signed MSIX, native DPAPI/NTFS, provider/crypto,
  traffic, memory and physical interop remain manual/deferred.
  Hosted CI run `29718751890` passed all four required jobs; PR #294 merged
  exact head `684734175473b3c4fc28a305431547fc1d0a3b62` to `main` as
  `c5eede96a18e19703c503ca32256e87a2b932838`.
- Windows protected-media playback producer: exact commit
  `532774a1c37778a744acba53e897c6308435ebc0` adds a production-dark exact-route
  manifest/envelope/record authentication boundary, witnessed group and
  bounded-history revalidation around every range, defensive route ownership,
  ciphertext-only cache and authenticated-reader injection into the existing
  bounded player. Exact `VariantURL` now participates in cache authority;
  actors sharing a root serialize read-merge-write, preserve tombstones as a
  monotonic union and use unique temp files. A separate route-scoped monotonic
  revocation marker survives parallel actors and restart. Protected focused 10
  scenarios plus race, stream regressions plus race, full Go plus race, vet,
  acceptance 210/210 and automated 16/16 passed; exact final manifest is
  `.temp/acceptance/20260720T060457Z/manifest.json`. Independent Claude Fable 5
  max run `RUN-260720-a152a9` reproduced full/race/vet, Windows cross-builds,
  acceptance 210/210 and a fresh automated 16/16, exercised an adversarial
  aliasing probe, recomputed all packet hashes and ACCEPTED exact producer SHA
  with zero open Critical/High/Medium findings. Signed MSIX, native DPAPI/NTFS/ACL, real
  provider/crypto/decoder, traffic, disk/log/memory/crash/swap/backup,
  hardware/audible and cross-platform interop evidence remains manual/deferred.
- Hosted CI run `29721560275` passed all four coordinator, NodeCore, Windows and
  packaged-probe jobs; PR #295 merged exact reviewed head to main as
  `e47eb6b583fa0319beee460b87397bdb75dbcf39`.
- Windows E2EE live-PTT producer: exact commit
  `aee07339bcfe014b39edac10734f713d11333792` adds the production-dark exact
  `BE` wire mirror, cross-process witnessed `live_ptt` generation reservation,
  shared epoch plus commit-digest AAD, retry-safe sealing on the existing
  transport worker, authentication before jitter and bounded replay/nonce/
  membership teardown. Defensive copies cover provider and caller aliasing;
  local repository revisions remain CAS-only and a two-installation fixture
  proves shared-context round trip under deliberately skewed revisions.
  Focused 11 scenarios plus race, live capture/receiver/node regressions plus
  race, full Go plus race, vet, acceptance 215/215, Windows amd64/arm64 blind
  compile and automated 16/16 passed; exact manifest is
  `.temp/acceptance/20260720T065018Z/manifest.json`. Claude Fable 5 max terminal
  completion run `RUN-260720-21d7d3` independently re-audited exact producer
  SHA after incomplete run `RUN-260720-c87e23`, recomputed all 11 hashes and
  repeated focused/full/race/vet/cross-build, acceptance 215/215 and a fresh
  synchronous 16/16 at `.temp/acceptance/20260720T071009Z/manifest.json`, then
  ACCEPTED with zero open Critical/High/Medium. Real traffic/audio/hardware/native/forensic
  evidence remains manual/deferred in `EPIC-260714-th54l3`.
- Hosted CI run `29724092583` passed all four coordinator, NodeCore, Windows and
  packaged-probe jobs; PR #296 merged exact reviewed head to main as
  `c11352b2676e746d18a28e74ac743fc799efeaa0`.
- macOS encrypted-media client-path exact producer commit
  `3a64b1808ce990fbef2cfb737839a15cbd0f6cbb` is accepted. The production-dark SwiftUI model and surface keep the
  selected protected path blocked instead of silently selecting plaintext,
  expose verified-device revoke, current-epoch transfer or explicit user-held
  recovery, object/epoch/device-bound history grants, irrecoverable-history
  warning and separate metadata-only versus decrypted-evidence report consent.
  A dormant composition constructs the accepted key-state, send, playback and
  live services from exactly one `MacE2EEKeyStateRepository` and retains a
  required cross-process ownership lease; `main.swift`, capability, provider,
  suite and container remain dark. Focused Swift passed 6/6, full Swift passed
  356/356, acceptance contract tests passed 242/242 and the automated harness
  passed 16/16 at
  `.temp/acceptance/20260720T074006Z/manifest.json`. Real signed/notarized app,
  Keychain/provider/codec, device transfer, physical audio/hardware,
  traffic/memory/crash and moderation-storage evidence remain manual/deferred
  in `EPIC-260714-th54l3`.
- Independent Claude Fable 5 max run `RUN-260720-c23a33` reproduced focused
  Swift 6/6, full Swift 356/356 across 57 suites, task acceptance 5/5 including
  four fail-closed mutations, full automated 16/16 at
  `.temp/acceptance/20260720T080331Z/manifest.json`, and a release build. It
  confirmed the path remains production-dark, one repository and retained
  abstract cross-process lease compose send/playback/live without generation
  double reservation, protected status and commands fail closed without
  plaintext downgrade, recovery/grant/report-consent boundaries match the
  accepted contracts, and UI state contains no secrets or rendered stable
  identifiers. Verdict: ACCEPTED with zero open Critical/High/Medium. Two Low
  findings track a cosmetic tautological normalization branch and the dormant
  composition throw path lacking an executable-target test before future
  runtime enablement; neither changes acceptance or manual deferrals.
- Hosted CI run `29726989792` passed all four coordinator, NodeCore, Windows and
  packaged-probe jobs after one diagnostic rerun of a non-reproducing
  `node-core` failure; the exact CI command passed locally on clean PR head and
  the rerun passed in 2m45s without code changes. PR #297 merged to `main` as
  `d265228f67858111276a8b466d6c0eb50ab66e54`.
- Windows encrypted-media client-path producer implementation is complete and
  awaiting exact-SHA independent review. The production-dark Go model exposes
  honest encrypted/plaintext status, verified-device revoke, current-epoch
  transfer or explicit user-held recovery, object/epoch/device-bound history
  grants, irrecoverable-history warnings and separate metadata-only versus
  decrypted-evidence consent. Its dormant composition passes exactly one
  accepted `WindowsE2EEKeyStateRepository` to the accepted send, playback and
  live services. `main.go`, runtime capability, provider, suite and container
  selection remain dark. Focused and full Go tests, vet, standalone race,
  Windows amd64/arm64 cross-builds, the validator and five focused acceptance
  tests pass; aggregate acceptance contract discovery also passes. One
  unrelated capture-workflow timing test flaked once in the aggregate race
  stage and passed immediately on an exact standalone race rerun. No acceptance
  credit is taken until Claude Fable 5 max completes exact-SHA review with zero
  open Critical/High/Medium finding. Real signed-MSIX/native-DPAPI, device,
  audio, accessibility and forensic evidence stays in `EPIC-260714-th54l3`.
- Current deferred coding line: E2EE continues with `TASK-260712-2q4jbu`, limited
  to the production-dark Windows encrypted-media client integration path. Every later E2EE implementation
  task lives in `EPIC-260716-3qsztl`; `TASK-260712-1ulshp` is retained there
  as well and cannot be self-certified by the implementation session.
- Most recently accepted: `TASK-260712-2nppt6` —
  macos-encrypted-media-client-path (dormant engineering scope only)
- Current branch: `feat/task-260712-2q4jbu`
- Current review evidence:
  macOS encrypted-media client-path was accepted on exact producer `3a64b18`
  by Claude Fable 5 max run `RUN-260720-c23a33`; verdict resource is
  `TASK-260712-2nppt6_independent-review-verdict.md`.
  Windows E2EE live PTT was accepted on exact producer `aee0733` by Claude
  Fable 5 max terminal run `RUN-260720-21d7d3`; verdict resource is
  `TASK-260712-39vjzd_independent-review-verdict.md`. Initial run
  `RUN-260720-c87e23` completed the audit but exited before its background
  harness and produced no terminal verdict, so no credit was taken from it.
  Windows protected playback was accepted by run
  `RUN-260720-a152a9` on exact producer commit `532774a`; verdict resource is
  `TASK-260712-1u57qz_independent-review-verdict.md`. The preceding accepted
  evidence is `TASK-260712-28zhpl_re-review-verdict-b2a4af6.md` from run
  `RUN-260720-1e8fa2` on exact producer rework commit `b2a4af6`; initial run
  `RUN-260720-6ead84` supplied the two now-closed lifecycle repros. The rejected
  first-pass verdict was run `RUN-260720-db683a`; the preceding playback
  review was run `RUN-260720-cf2797`. The preceding
  playback review run was `RUN-260720-2f341a`; send review was
  `RUN-260720-cc3c8d`; recovery was `RUN-260720-6193e1`; report evidence was
  `RUN-260720-65a670`; the preceding
  Windows key-state review was `RUN-260719-c050cd`; macOS key-state review was
  `RUN-260719-20ab4a`; opaque-router completion was `RUN-260719-91776a`. The Store-package run `RUN-260719-85bf38`
  accepted exact `e3bf985` for engineering scope in
  `TASK-260712-2s4e9p_engineering-review-verdict.md`; manual screenshots/WACK
  and exact Partner Center/IARC owner inputs remain open in
  `TASK-260712-e5mfqj` and `TASK-260715-24ube9`.
- Current deferred owner gates in `EPIC-260714-zmnd4n` are
  `TASK-260716-tlxe3s` for the exact codec/legal/supply-chain decision and
  `TASK-260716-3voo6j` for independent streamed-performance acceptance. The
  latter consumes physical evidence from manual tasks `TASK-260712-1fpb9q`
  and `TASK-260712-2bdi4a`. Production playback and Phase 2 promotion stay
  blocked. Air production and promotion are additionally blocked on manual
  tasks `TASK-260712-21kz3b` and `TASK-260712-3qybi2` plus independent approval
  `TASK-260716-19g4gd`. Target/range production and promotion are additionally
  blocked on manual tasks `TASK-260712-3u5cdn` and `TASK-260712-3qybi2` plus
  independent approval `TASK-260716-2l5j1a`. Reversible engineering continues
  under the owner-approved best-effort rule. Phase 3 realtime activation and
  promotion are additionally blocked on manual C1-C3 task
  `TASK-260712-flaiie` and implementation-independent approval
  `TASK-260717-3dbi2v`. Phase 3 automation activation and promotion are
  additionally blocked on manual C7 task `TASK-260712-1gyohk` and
  implementation-independent approval `TASK-260717-1pyg62`. Phase 3 E2EE,
  soundboard, automation and promotion are additionally blocked on live policy
  and mailbox evidence, exact Partner Center evidence and implementation-
  independent privacy/Store approval `TASK-260717-35bll1`; that task consumes
  `TASK-260714-200ib8`, `TASK-260715-24ube9` and the later disclosure packet
  `TASK-260712-3b7bp4`. Phase 3 beta and promotion are additionally blocked on
  manual rollout/rollback/recovery task `TASK-260712-30xwu2`, deferred E2EE and
  implementation-independent migration/recovery approval `TASK-260717-1sgb5r`.
- Current external-input gate: all seven legal/operations groups are approved
  by Ivan Oparin; exact head `3b12371` passed all four hosted jobs in run
  `29338589269`; tracking head `5af1b56` passed all four jobs in run
  `29339017452`. PR #29 landed at merge
  `e588fc9b727d6264c289f69cc97ea77e4987f939`.
- Current external-action ledger: `EPIC-260714-zmnd4n`. DNS inspection found
  no MX for `barycenter.live`; provider-side routing and synthetic delivery for
  the approved mailboxes are tracked as `TASK-260714-200ib8`. On 2026-07-19
  Ivan Oparin approved Cloudflare Email Routing to one private verified
  Ivan-controlled destination as the default approach; the destination,
  provider mutation and delivery evidence remain open. The same approval
  accepted the non-implementing protocol-reviewer selection default in
  `TASK-260715-3ffm3r` and the dark-only bundled FFmpeg candidate default in
  `TASK-260716-tlxe3s`. The protocol and realtime-audio reviewers have since
  returned independent APPROVE verdicts; the other withheld external verdicts
  and production activation remain open. Store submission remains fail-closed.
- Accepted overall: 182 / 205 tasks (88.8%); 23 remain
- Engineering progress: 182 / 186 tasks (approximately 97.8%); 4 remain
- Manual-test progress: 0 / 19 tasks; all remain deferred
- State: the physical H00-H17 task and 18 later real-app, platform,
  production-shaped or beta acceptance tasks were moved to
  `EPIC-260714-th54l3`. Their evidence is not claimed passed and they no longer
  block best-effort coding, unit tests, deterministic integration tests, CI,
  packaging or engineering review. PR #10 landed this boundary and the Windows
  evidence harness on `main` at `06a06c099ed5b4f37f5e2dd3648772ffd041dfd9`.
  `TASK-260712-z6h6wh` landed through PR #11 at merge commit `31bbeb9`;
  `TASK-260712-1bnos4` landed through PR #12 at merge commit `050c979`;
  `TASK-260712-2af2dp` landed through PR #13 at merge commit `451e50b`;
  `TASK-260712-1sae4q` landed through PR #14 at merge commit `fe8e73c`; strict
  `TASK-260712-3mcof4` landed through PR #15 at merge commit `0f3148a`;
  `TASK-260712-12ojcb` landed through PR #16 at merge commit `0d6863c`;
  `TASK-260712-gj0cko` landed through PR #17 at merge commit `9f2aea8`;
  `TASK-260712-3huupe` landed through PR #18 at merge commit `cfe12ed`;
  `TASK-260712-jolzhh` landed through PR #19 at merge commit `c4cb324`;
  `TASK-260712-51y5k9` landed through PR #20 at merge commit `2aa97c2` after
  hosted CI runs `29314060965` and `29314299856`; `TASK-260712-1aprcb` landed
  through PR #21 at merge commit `35d9974` after hosted CI runs `29315987760`,
  `29316416647` and `29316678680`; `TASK-260712-1g70av` landed through PR #22
  at merge commit `2473020` after hosted CI runs `29318171135`, `29318440712`
  and `29318696473`. `TASK-260712-2qpp6w` is accepted on exact code head
  `4d737bcdbb0c40c53b8d2d64651756b4b9b077b2`; all four hosted jobs passed in
  runs `29321759958`, `29322386396` and tracking run `29322606238`; PR #23
  landed exact head through merge `30f1c552c9824934922becab4637c34746d190dc`.
  `TASK-260712-26ip33` landed through PR #24 at merge
  `0b54899073e4dc4948b248f7c77666e7151f5459` after exact code and tracking CI
  runs `29324579129` and `29324846258`. `TASK-260712-2bbz13` is accepted on
  exact engineering code head `219306ceda548b64a6bb72e279c9ac9da4e65313`;
  all four hosted jobs passed in run `29326895259`, and tracking head `c85e8ad`
  passed run `29327302466`. PR #25 landed at merge
  `0c1e1946ff692aa553c19ca6bf7328150d1a24b8`; strict execution has advanced to
  `TASK-260712-31vvjt` from that synchronized `main`. `TASK-260712-31vvjt` is
  accepted on exact engineering code head
  `d0e1b925aa72048c243739d61bcf61fb51443ab7`; all four hosted jobs passed in
  run `29331940948`; tracking head `baf8210` passed all four jobs in run
  `29332298395`. PR #26 landed at merge
  `8d2b7d3825536ed9dc732f1e86040edc227a7acf`, and strict execution advanced to
  `TASK-260712-2qc27p` from that synchronized `main`. `TASK-260712-2qc27p` is
  accepted on exact engineering code head
  `c60bd99ed4717a62b69a10338e5b13b39001e419`; all four hosted jobs passed in
  run `29333494719`, and tracking head `d45f535` passed all four jobs in run
  `29333795623`. PR #27 landed at merge
  `70f26072cb36f3ee6e5cd4358bdded2bf98b7214`, and strict execution advanced to
  `TASK-260712-2cdjq8` from that synchronized `main`. `TASK-260712-2cdjq8` is
  accepted on exact documentation/code head
  `cd234c913634db2fef5bbfcd866e8298e45f23cb`; all four hosted jobs passed in
  run `29334550550`, and tracking head `e715202` passed all four jobs in run
  `29334859168`. PR #28 landed at merge
  `3c720410fb54ed92ecc16f905d170d4f411d1b93`, and strict execution advanced to
  `TASK-260712-16zfvu` from that synchronized `main`.

Checkpoint 2026-07-14: `TASK-260712-2kec2s` implements the least-privilege
moderation control plane on exact engineering code head
`2a0b1352bd79ef8b51863ba5f2ab77188d66ff22`. Additive persistence covers
hashed and independently scoped operator credentials, privacy-safe user
reports, immutable accepted-target snapshots, time-limited digest-verified
evidence, crash-resumable one-decision state and append-only audit records.
Actor APIs permit only accessible foreign media and keep operator credentials
separate; operator actions reuse canonical block, media lifecycle, credential
revocation, scheduler cancellation and live disconnect paths. Report rate
state, evidence retention, idempotence, audit tamper resistance and exact
previous-head migration/rollback are covered deterministically. Local Go vet,
full tests, focused race, full pinned rollback matrix, Windows vet/test/cross-
build and Swift release build passed. Local Swift tests were unavailable under
the standalone CommandLineTools installation; hosted `node-core` ran them
successfully. All four hosted jobs, including the signed packaged probe,
passed in run `29342009648`; tracking head `a268ebb` passed all four jobs in
run `29342525843`. No physical-app or real-hardware result is claimed. Progress
is 31/205 overall and 31/186 engineering. PR #30 landed at merge
`c6f1afdb1040bc654b18e324f71b71fd524ca1e7`, and strict execution advanced to
`TASK-260712-g9ycx5` from synchronized `main`.

Checkpoint 2026-07-14: `TASK-260712-g9ycx5` freezes the current official
Microsoft Store and IARC baseline on exact engineering code head
`f0bcacef669dc0c8cfeec694d1e5d0323abbef83`. Microsoft Store Policies remain
version 7.19, published 2025-09-10 and effective 2025-10-14. Human-readable and
strict machine-readable matrices distinguish mandatory rules from guidance,
map 10.1, 10.3, 10.5, 10.6, 10.7, 11.11, 11.12 and current listing, asset,
WACK and IARC requirements to concrete implementation/evidence tasks, and
explain both the credential-free reviewer path and the still-mandatory 10.3.2
coordinator availability. The six-shot EN/RU corrective set is specified, but
real-app capture stays honestly deferred to manual `TASK-260712-e5mfqj`.
Repository-only July finding summaries are not promoted into official claims:
the raw report is absent, its recorded dates conflict, and `10.1.1.3` is
treated as a finding label anchored to public 10.1/10.1.1. A strict Go
validator plus Store workflow gate now requires a tag-bound policy verification
no older than 24 hours and task IDs for every delta; the initial checked-in
record is deliberately `hold`. Local full/vet/race/platform gates passed, and
hosted run `29343948310` passed all four jobs. Progress is 32/205 overall and
32/186 engineering; tracking head `869b551436e8439a0023f0467246c64a4d7a6be7`
passed all four jobs in run `29344280643`. PR #31 landed at merge
`d40b754493b78bb58c24b4fc759312c4a0463533`, and strict execution advanced to
`TASK-260712-1epb3a` from synchronized `main`.

Checkpoint 2026-07-14: `TASK-260712-1epb3a` authors versioned Privacy, Terms,
Content Guidelines and upload/recording-rights sources in semantically aligned
English and Russian on exact engineering code head
`27c19cdad6711f6790594a63fa4ec0a51687f062`. Forty-four stable section IDs map
the complete specification 15.1/15.2 disclosure set to approved legal inputs,
shipped controls, explicit product limitations and current primary Microsoft,
Telegram, Spotify, FTC, EU and California sources. The pack says plainly that
Phase 1 is accountless, target-limited and not E2EE; documents seven-day clip
and backup bounds, report evidence, asynchronous deletion, recipient-copy
limits and optional integrations; and withdraws the obsolete Store claim that
Pulsar collects no personal data. A strict Go validator checks exact document
hashes, EN/RU section parity, source authority, public facts, traceability and
surface owners. Store submit now fails closed until Ivan Oparin approves these
exact authored bytes: approved defaults are incorporated, but the exact-content
decision remains honestly `hold`. Local full/vet/race/platform gates passed;
hosted run `29345880750` passed all four jobs. No public-URL, real-app or
physical-hardware result is claimed. Progress is 33/205 overall and 33/186
engineering. Tracking head `388f71c7844566f4c0a3f1d989627d9f821ba122`
passed all four hosted jobs in run `29346224420`; PR #32 landed at merge
`f1048c280aa7bdf6bfd92c7b2a971fc9dc027983`, and strict execution advanced to
`TASK-260712-1x0lot` from synchronized `main`.

Checkpoint 2026-07-14 (in progress): `TASK-260712-1x0lot` stages a
deterministic policy/support publication pipeline on Barycenter head
`43c0bd992e25c1e85aba6b7a086a94dad378eb35` in draft PR #33. It extends the
exact-hash pack with five EN/RU support sections; generates 10 stable and 10
immutable versioned HTML routes with locale switches, stable anchors, source
and rendered hashes; wires macOS, Windows, Telegram and Store source metadata;
and adds fail-closed Store/uptime live checks plus cache/rollback documentation.
Generated `pulsar-site` head
`1316a268ac025570a62f9d86a83e56146b5e3779` is staged in draft PR #1 and pins
the exact Barycenter source commit. Local coordinator full/vet/race, Windows
vet/race/amd64+arm64 cross-build, Swift release build, deterministic 33-file
regeneration, 20-route local serving, JSON/YAML/diff and board validation pass.
Cloudflare preview success is not a production publication. Ivan Oparin then
explicitly approved the exact ten source hashes from immutable source commit
`43c0bd992e25c1e85aba6b7a086a94dad378eb35` at
`2026-07-14T20:09:26+04:00`; the source pack is now `proceed`. Production
deployment and live hash/cache verification remain before task acceptance, so
progress is still 33/205 overall and 33/186 engineering.

Checkpoint 2026-07-14 (accepted): `TASK-260712-1x0lot` published the exact
owner-approved EN/RU policy/support bundle. Barycenter engineering head
`2da485fa2a094daec7622a14822d45ecfc2338db` passed all four hosted jobs in run
`29348947568`. The first production probe correctly rejected clean-path empty
responses and Cloudflare email/body rewriting; explicit 308 redirects and
`no-transform` stable/immutable cache directives fixed both defects without
weakening the exact-byte validator. `pulsar-site` PR #2 landed the corrected
bundle on `main` at `6322e28a145b6c563184899fe84da81bc0733287` with Cloudflare
Pages and exact-upstream-bundle checks green. The production deployment
manifest names the exact upstream head and source-pack SHA-256
`0626909361f478c372243af1a488ddbccfc3dad33493f8c3f4f8e12b414aabe7`;
`policy-site-check --require-proceed --live` then matched all 20 page hashes,
redirects and cache contracts. No packaged-app click is claimed. Progress is
34/205 overall and 34/186 engineering; strict execution advances to
`TASK-260712-3t9nr8`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-3t9nr8` now has an
accountable moderation operations contract and runbook for Ivan Oparin's
approved primary/backup/escalation roles, GMT+4 coverage, ordinary and urgent
targets, reporter-safe intake, verified Microsoft requests, evidence privacy,
retention, backups, operator credential issue/revoke and honest correction
boundaries. A report-scoped operator API exports append-only content-free audit
events under `list` authority without exposing report text, evidence, storage
identity, tokens or paths. CI validates the contract; Store submit additionally
requires live mailbox readiness. DNS inspection found no MX records for
`barycenter.live`, so delivery is not invented: strict readiness fails closed
and provider routing is recorded as external owner task `TASK-260714-200ib8`
under `EPIC-260714-zmnd4n`. Coordinator full vet/tests, targeted moderation
race, exact moderation predecessor rollback, Windows vet/tests/cross-build,
Swift release build, JSON and board validation pass. A broader coordinator
race run passed `internal/store` but hit an unrelated Telegram SQLite-busy
flaky test; the changed HTTP moderation path passed independently under race.
Exact engineering head `9bcce41920a6c64eb823e41de2f691db456bd849`
passed all four hosted jobs in run `29350324690`. The real-delivery checklist
item remains honestly unchecked and transferred to `TASK-260714-200ib8`; Store
submission is fail-closed until it passes. Under the owner-approved external-
question boundary, engineering progress is 35/205 overall and 35/186
engineering. Tracking head `3c87a26` passed all four hosted jobs in run
`29350566965`; PR #34 landed at merge
`74b4e04e9386c9834e435cf4aaf46c129626a278`, and strict execution advanced to
`TASK-260712-1hqiek` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-1hqiek` freezes one immutable
`MixerControlParameters` handoff and the same generation-safe
prepared/armed/playing/cancelling/terminal lifecycle on macOS and Windows.
Newer prepare cannot steal active render ownership; a newer sender-delete
waits for mixer cancellation acknowledgement before publishing its terminal
tombstone, and late callbacks cannot revive an old generation. macOS decodes
into a preallocated PCM buffer, publishes gain work through a fixed SPSC queue,
uses atomic telemetry and dispatches first-sample notification off-render.
Windows publishes immutable voice/click state atomically, keeps gain and
amplitude reads lock-free and routes legacy voice completion through a
pre-created bounded dispatcher. Static source/AST tests reject allocation,
goroutine creation, waits and blocking lock calls in the checked callbacks;
the separate legacy `play_voice` path remains documented and operational.
Local coordinator vet/tests, Windows full/race/vet and amd64/arm64 cross-builds,
Swift release build and board validation passed. Exact engineering code head
`8521b84` passed coordinator, authoritative hosted Swift tests, Windows and
signed packaged-probe jobs in run `29351870335`. No real-device or audible
quality result is claimed. Progress is 36/205 overall and 36/186 engineering;
tracking head `2e76d03` passed all four hosted jobs in run `29352120463`. PR
#35 landed at merge `523264c3c5904c3cb0d0e10e9ee155d042c592cc`, and strict
execution advanced to `TASK-260712-1viwvi` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-1viwvi` replaces the Windows
prepared-only adapter with an Engine-backed `overlay_mix_v1` mixer. Decoding
and allocation remain off-render; the callback continuously consumes the main
ring through pre-duck, additive playback, cancellation and release. The frozen
`-12 dB` target, `250 ms` attack, `600 ms` release, absolute T-minus-250 ms
pre-duck and `-1 dBFS` post-mix ceiling execute before final master gain. Late
but valid scheduling catches the envelope up to absolute time. Wire `fade_ms`
ramps the clip without a gain step, and cancellation is acknowledged only after
both overlay fade and duck release are terminal. Aggregate overlay-frame,
limiter-hit, underrun and ring-fill telemetry exposes no identity or PCM.
Deterministic tests cover a ten-second overlay with exact continuous main
consumption/zero position error, absent-main gain, limiter action, cancellation,
zero render allocations, twenty FIFO handoffs and full MediaClipClient receipt
flow. Local coordinator vet/tests, Windows vet/full/race and amd64/arm64
cross-builds, Swift release build and board validation passed. Exact engineering
head `dac4310` passed all four hosted jobs in run `29353275479`. No physical or
audible Windows result is claimed. Progress is 37/205 overall and 37/186
engineering; tracking head `4d801b0` passed all four hosted jobs in run
`29353504276`. PR #36 landed at merge
`3db0d01967db186197384310c6022914f75cfdc5`, and strict execution advanced to
`TASK-260712-2zbmq4` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-2zbmq4` replaces the macOS
prepared-only adapter with a real additive `AVAudioPlayerNode` overlay branch.
Preparation fully decodes and converts clips to the engine's 44.1 kHz stereo
format off-render; arming schedules the immutable buffer at an absolute host
time. The source callback continues its unconditional ring read while a serial
control queue drives T-minus-250 ms pre-duck, late-envelope catch-up, natural
release and raised-cosine cancellation. The exact program-mixer order places
Apple's DynamicsProcessor before final local master gain, with a `-1.1 dB`
threshold plus `0.1 dB` headroom freezing the pre-master ceiling at `-1 dBFS`.
Cleanup releases ducking and resets the overlay node for immediate reuse;
signed host-time arithmetic safely handles valid late starts. Aggregate frame,
limiter-hit, ring-fill and underrun telemetry exposes neither PCM nor identity.
Deterministic coverage includes off-render 48-to-44.1 kHz conversion, exact
pre-duck/default ramps, a ten-second overlay, graph reuse, cancellation, source
callback safety and gain order. Full local Xcode testing passed 154 tests;
release build, coordinator tests, Windows race tests and Windows cross-build
also passed. Exact engineering head `731c83d` passed all four hosted jobs in
run `29354780914`. No physical macOS playback, audible-quality, hardware timing
or real position-error result is claimed; those remain in
`EPIC-260714-th54l3`. Progress is 38/205 overall and 38/186 engineering.
Tracking head `9cc63b0` passed all four hosted jobs in run `29355049817`. PR
#37 landed at merge `f77b1d512d97c18865d176c91e808de624cb9c23`, and strict
execution advanced to `TASK-260712-1g6lk8` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-1g6lk8` adds a single-owner,
render-safe Windows interrupt branch. The engine applies the wire-controlled
250 ms pre-fade, stops consuming the main ring at exact `T`, renders the
prepared replacement through the existing limiter and holds the program until
the off-render resume handshake completes. `Player` snapshots the audible
anchor as provider/extrapolated position minus queued ring frames, binds it to
the exact element/load generation, pauses the daemon, clears buffered tail,
then seeks/resumes once behind the 120 ms default fade-in. Stop, load, voice,
wait and reconnect invalidate stale tokens and prepared media generations;
unavailable exact ownership returns `interrupt_capability_lost` without
overlay or `after_current` fallback. Deterministic coverage proves ring stop,
limiting, exact buffered anchor, natural/cancel resume-once, stale-stop and
reconnect behavior plus render-boundary safety. Local unit/race/vet tests,
Windows amd64 cross-build, coordinator tests and macOS release build passed.
Exact engineering head `a29db301e139e46f00154a29c2411e8578268eab`
passed all four hosted jobs in run `29356446731`. Physical Windows A4 timing
and audible evidence remain unclaimed in `EPIC-260714-th54l3`. Progress is
39/205 overall and 39/186 engineering. Tracking head `6b9d483` passed all four
hosted jobs in run `29356669431`. PR #38 landed at merge
`adaac8cbf9949fded51c95a28b906db420c65f99`, and strict execution advanced to
`TASK-260712-8mwyiv` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-8mwyiv` extends the existing
prepared macOS `AVAudioPlayerNode` mixer with exact interrupt ownership. The
capability is advertised only after `PlayerCore` binds its controller. The
mixer drives the wire-controlled 250 ms pre-fade, captures the audible anchor
as extrapolated provider position minus queued ring tail at `T`, clears the
buffer and pauses the provider off-render. Natural completion and cancellation
serialize one exact seek/resume behind the provider command barrier and apply
the 120 ms default fade-in; later load, pause, seek and stop commands wait for
that barrier so an old provider command cannot overtake a new generation.
Reconnect, stop and provider restart reset prepared generations and invalidate
tokens. Cancel racing resume produces one terminal cancellation. Unsupported
ownership fails as `interrupt_capability_lost` with no fallback; resume failure
becomes `media_failed(stage=play, code=audio_graph_failed)` instead of a false
completed playback, and the graph remains reusable. Deterministic tests cover
the 9,950 ms anchor from 10,000 ms provider minus 50 ms ring tail, pre-fade,
resume-once, failure recovery, cancel/resume races and reconnect late-callback
suppression. The full 162-test Swift suite, 20 repeated focused runs, release
build, coordinator tests, Windows race tests and Windows cross-build passed.
Exact engineering head `2a06f2f55379a5aeeb5e1f27fb9733adc7e01e4f`
passed all four hosted jobs in run `29357878003`. Physical macOS A4 remains
unclaimed in `EPIC-260714-th54l3`. Progress is 40/205 overall and 40/186
engineering. Tracking head `34f1abe` passed all four hosted jobs in run
`29358110382`. PR #39 landed at merge
`a21c79bded4605b40781fdfdb1954bdadf4d1c29`, and strict execution advanced to
`TASK-260712-3d6cnn` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-3d6cnn` freezes the common
automated A3/A4 engineering gate without claiming real-app hardware results.
Windows and macOS fixtures assert pre-duck and release ramps, exact gain order,
limiter behavior and hit counters, 200/500 ms deterministic report bounds,
audible interrupt anchor, resume-once, stale-generation rejection and active
`media_deleted` recovery. Both implementations run 100 sequential overlays;
the Windows fixture bounds retained heap growth and the macOS fixture proves
prepared owners release. Render-source guards reject allocation, I/O, waits,
locks and sleeps; Windows additionally measures zero allocations. The maximum
180-second stereo PCM fixture remains one 63,504,000-byte backing buffer on
both platforms, and macOS now rejects the shared P1 maximum before fetch just
like Windows. Local Go, race, Windows cross-build, coordinator, 165-test Swift
suite, focused repetitions and Swift release build passed. Exact engineering
head `f45de46b3b8482620ddc057795383f0180026759` passed all four hosted jobs in
run `29358958855`. Audible quality, real Spotify timing, routes and physical
OS/device matrices remain unclaimed in `EPIC-260714-th54l3`. Progress is
41/205 overall and 41/186 engineering. Tracking head `94da92b` passed all four
hosted jobs in run `29359218926`. PR #40 landed at merge
`8cc171f7429f25aac68c272b9a96b2a388be4b81`, and strict execution advanced to
`TASK-260712-3coble` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-3coble` freezes normative
`p1-history-presence-telegram-v1` for every downstream surface. The contract
defines exact history list/detail authorization, stable snapshot cursors,
30-day receipt/content action boundaries, sanitized presence, optimistic
local/orbit DND ownership and precedence, opaque actor/orbit block references
and exact receipt reasons. Telegram is clip-only at 20 MiB source, 34 MiB
canonical and 180 seconds; Phase-2 track paths fail honestly. A 36-byte opaque
callback stores only a keyed hash, expires after 15 minutes, deduplicates
query IDs for 24 hours and binds current actor/role/orbit/chat/message/media
generation. Ready media creates its legacy `after_current` immediately; a
valid pre-start click atomically cancels/replaces it with one new trusted
acceptance time, while interrupt confirmation leaves the default queued and a
start race produces exactly one audible transmission. Eight JSON examples and
critical decisions are guarded by a coordinator document test; the corrected
sequence diagram now shows the real no-decision-window race. Coordinator full
and race tests, Windows tests/cross-build, 165 Swift tests and release build
passed. Exact engineering head `dfefae6680f2241acd51dcd9c3d4e7986723b967`
passed all four hosted jobs in run `29360209758`. No hardware result is
claimed. Progress is 42/205 overall and 42/186 engineering. Tracking head
`177ca51` passed all four hosted jobs in run `29360440103`. PR #41 landed at
merge `b9e138fbc5eda8fffe5ba733ec0b750a72b828b4`, and strict execution advanced
to `TASK-260712-1gx6mh` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-1gx6mh` adds one transport-
neutral `key/en/ru` presentation model for HTTP, Windows/macOS consumers and
Telegram. It covers sender/member/origin, direct and linked targets, all P1
audiences, include-origin, requested/effective delivery, downgrade and
interrupt confirmation, media/aggregate/target statuses and all 38 frozen
receipt reasons. Unsafe numeric Telegram/database IDs, raw slots, composite
peers and typed internal IDs fall back to stable human copy rather than being
echoed. The legacy Telegram `/home`, `/status`, queue, voice-target and provider
error paths now consume this model and escape only at the transport boundary;
the old raw `a`, `b` and `42:a` presentation is gone. A sorted RU/EN SHA-256
golden, exhaustive enum inventory, direct/approach/missing metadata fixtures,
duplicate/transport wording guard and real `/home` missing-name test are green.
Coordinator full/race, Windows test/cross-build, 165 Swift tests and release
build passed. Exact engineering head
`31024a2c089ad08e4359cf8843be037d14bc42eb` passed all four hosted jobs in run
`29361254030`. No hardware result is claimed. Progress is 43/205 overall and
43/186 engineering. Tracking head `bbd6c7a` passed all four hosted jobs in run
`29361470602`. PR #42 landed at merge
`c820a869c2d501f8b549a21a019e2cd9e0fc87e3`, and strict execution advanced to
`TASK-260712-3dmllz` from synchronized `main`.

Checkpoint 2026-07-14 (accepted): `TASK-260712-3dmllz` adds typed Telegram
callback, audio and document transport while preserving the legacy voice path.
Filename, MIME, duration and size remain hints: audio/documents enter the same
20 MiB bounded download and common `SubmitMedia` signature, decoded-duration
and canonical-size proof. Non-audio update shapes, media groups, actual
oversize and decoded Phase-2-only tracks receive stable non-disclosing replies.
Callback data is exactly a 36-byte `tg1_` opaque random reference indexed only
by HMAC-SHA-256; its server row binds actor, role, source orbit, chat/message,
original update, media generation, action and canonical options. Tokens expire
in 15 minutes, query IDs are HMAC-indexed and actor-bound for 24-hour replay,
failed actions remain retryable, and callbacks use a dedicated prompt queue
with terminal keyboard cleanup. Deterministic tests cover contradictory media
hints, unsupported shapes, forgery, expiry, cross-actor/role/orbit/message,
source-primary authorization, replay, retry, exact HTTP forms and redaction.
Local coordinator full/race, pinned rollback, Windows test/cross-build and Swift
release gates passed; hosted macOS Swift tests and all other jobs passed on
exact engineering head `773b417eaff6308fe3cfa14cbe3d80e812854e75`
in run `29362994920`. No real Telegram client, audible or hardware result is
claimed. Progress is 44/205 overall and 44/186 engineering. Tracking head
`c5bdf42abb51c4b5c23f66500b672cc3ad84771c` passed all four hosted jobs in
run `29363292374`. PR #43 landed at merge
`4f026a0e290e899a813b8cf2ab78ef9c5386d178`, and strict execution advanced to
`TASK-260712-1c1ska` from synchronized `main`.

Checkpoint 2026-07-14 (candidate): `TASK-260712-1c1ska` adds the privacy-
allowlisted `GET /v1/presence`, exact-installation local DND, primary-owned
orbit DND and opaque actor/orbit block controls. Presence resolves only the
caller orbit/current pairwise approach, validates the current socket binding,
becomes offline after 12 seconds, and exposes only output, sanitized playback,
effective DND and sorted capabilities. Heartbeats project the closed
`idle/main/interrupt/unknown` playback vocabulary and reset at reconnect.
Telegram `/status` no longer exposes speaker names, position, volume, RTT,
offsets or process/library versions. DND authorization, expected revision and
digest-only actor-scoped idempotency commit in one writer transaction; app and
verified Telegram identities call the same `ActorContext` policy service.
History-facing `ar_`/`or_` refs and public `bl_` ids keep internal identity out
of HTTP; new DND/block policy cancels only matching current-generation pending
or active work, and unblock never resurrects it. Coordinator vet/full/race,
exact previous-head rollback, Windows vet/tests, Swift release build, board
validation and diff checks pass. No real-app, audible, physical-device or
hardware result is claimed. Exact engineering head
`a65fc659e3ae389484163723aa63a3806f4b986d` passed all four hosted jobs in run
`29365735642`; the task is accepted. Progress is 45/205 overall and 45/186
engineering. Tracking head `f4f718b3cd54143d9a5c9d6c5a1e39fe46d724d0`
passed all four jobs in run `29365971499`; PR #44 landed at merge
`19ea2c1fd58dd066cbe0a217dd28972a4ff77a6b`, and strict execution advanced to
`TASK-260712-2hcq1g` from synchronized `main`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-2hcq1g` adds the common
actor-scoped Phase 1 history read model and strict `GET /v1/history` plus
`GET /v1/history/{history_item_id}` surfaces. Unlinked media maps only to
processing/ready/error and disappears after its first transmission; retained
transmissions expose requested/effective delivery, downgrade, compact and full
receipt counts, permitted exact target rows, content availability and ordered
current action hints without creating replay or an offline inbox. Source
control, verified Telegram, exact current target binding, foreign/left/revoked
and node-only credential boundaries are deterministic. Stateful 256-bit
digest-only cursors bind actor, credential plus authorization scope, filters,
upper/last keys and 24-hour expiry; blocked receipt scope is visible only to
the owning recipient. Coordinator vet/full tests, ten-pass focused race,
exact previous-HEAD rollback, moderation validation, Windows vet/tests/cross-
build, Swift release build, board validation and diff checks pass. The local
CommandLineTools image still cannot import the pre-existing Swift `Testing`
module, so hosted macOS CI remains authoritative. No real-app, Telegram-client,
audible or physical-hardware result is claimed. Exact engineering head
`742c1600aee20159a96b4a15dc20957c31edf9ed` passed all four hosted jobs in run
`29368167361`. Progress is 46/205 overall and 46/186 engineering. Tracking head
`835efb765f9ae49ab4b5984f03f884815c34c2d9` passed all four hosted jobs in run
`29368383324`; PR #45 landed at merge
`77cf82f81df624da267b855adca7ebfe1d239bea`, and strict execution advanced to
`TASK-260712-21ers7` from synchronized `main`.

Checkpoint 2026-07-15 (engineering candidate): `TASK-260712-21ers7` now commits
the Telegram voice default and its durable route together, retains the trusted
intake timestamp for FIFO while evaluating readiness policy after publication,
and routes every selected action through the common transmission resolver and
scheduler. Audio/document clips are explicit-only. Exact message-bound `tg1_`
callbacks are stored as HMAC digests with fresh Telegram `ActorContext`,
15-minute token expiry and 24-hour query replay. Replacement and cancellation
of a not-started default are one SQLite transaction; start-first returns
`too_late`, concurrent choices yield one replacement, overlay downgrade is
visible through the shared presentation model, and interrupt fallback requires
a second durable callback. Coordinator vet/full/full-race, focused routing race
x5, fault rollback, legacy FIFO, bot HTTP keyboard, Windows native/cross-build,
Swift release build, PlantUML render and board/diff gates are green. The local
Swift test runner still lacks the pre-existing `Testing` module; no real app,
Telegram client, audible or hardware evidence is claimed. Exact commit, hosted
Exact engineering head `8fc47cf75b0f1ba521e80bd9d8a42885edacb217`
passed all four hosted jobs in run `29370460972`; the best-effort engineering
scope is accepted. Progress is 47/205 overall and 47/186 engineering. PR #46
tracking head `a9c6defb8def8aea277e24ece687ce9377c1e150` passed all four
jobs in run `29370645888`; PR #46 landed at merge
`912d08018cccda0589d5de7356bb8af8a20fd6f1`, and strict execution advanced to
`TASK-260712-3e4p0c` from synchronized `main`.

Checkpoint 2026-07-15 (engineering candidate): `TASK-260712-3e4p0c` adds one
transport-neutral history command service for application bearer and verified
Telegram identities. Strict `POST /v1/history/{history_item_id}/actions/...`
routes expose replay, owner delete, exact-target report and actor/orbit block
without accepting a client media ID, acceptance time or old target snapshot.
Replay uses a new coordinator `accepted_at` and the common current audience,
binding, presence, capability, DND and block resolver; same-key retries return
the existing transmission after later deletion, while a new request cannot
revive deleted or expired content. Delete reuses the audited media tombstone
and durable cancellation outbox, report reuses moderation evidence/rate-limit/
audit, and block reuses viewer-bound subject refs, role policy, idempotency and
active cancellation enforcement. App and Telegram owner paths share the same
service; revoked, departed, node-only, foreign and racing callers do not gain
authority. Coordinator vet/full tests and focused full race, pinned previous-
head compatibility, moderation operations validation, Windows vet/native/
cross-test compilation, Swift release build, both PlantUML renders and diff
checks are green. No real client, audible, physical-hardware or Phase 2 inbox
evidence is claimed; those remain in `EPIC-260714-th54l3`. Exact engineering
head `04f2b20c33b9af464e155b720f45838f70497ade` passed all four hosted jobs
in run `29372823415`; the best-effort engineering scope is accepted. Progress
is 48/205 overall and 48/186 engineering. Tracking head
`75cbb5b9f4ab1077691baa6d0c900ff2d208f343` passed all four jobs in run
`29373039104`; PR #47 landed at merge
`6df7ab4932e0b3fc8629ce0a92f924d34c78b557`, and strict execution advanced to
`TASK-260712-3d0zgu` from synchronized `main`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-3d0zgu` maps the complete
Telegram/history/presence regression contract to deterministic unit, SQLite,
HTTP and coordinator fake-transport evidence. The review added direct proof of
trusted no-action FIFO with zero synthetic decision wait, cross-user callback
query isolation, mixed-capability whole-target downgrade, pairwise target
naming and exact DND/block/downgrade copy. It also found and removed the final
private RU-only Telegram callback answer switch: `callback.*` semantic keys and
exact EN/RU text now live in the shared presentation catalog consumed by both
app and bot surfaces. Existing forgery/expiry/group-role, duplicate/start race,
attachment-proof, history tenant/action authorization, presence sanitization
and layered DND/block tests are linked in one acceptance matrix. Coordinator
vet/full/race, pinned previous-head compatibility, moderation operations,
Windows vet/tests/amd64+arm64 builds, Swift release build, all three PlantUML
renders, diff and board validation passed. Exact engineering head
`24a043e4794da90bccc22492269ed8fd699226a6` passed all four hosted jobs in run
`29373913897`; the best-effort engineering scope is accepted. No real Telegram
client, app, audible playback, packaged-device or physical-hardware result is
claimed; those remain in `EPIC-260714-th54l3`. Progress is 49/205 overall and
49/186 engineering. Tracking head
`17be7458f7829485d0297efb4232a53d79b6e1db` passed all four hosted jobs in run
`29374119932`; PR #48 landed at merge
`b6e49cb111c316d530161fff7e70c2a8420906b0`, and strict execution advanced to
`TASK-260712-1f9jtm` from synchronized `main`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-1f9jtm` publishes the durable
Phase 1 Telegram/history/presence rollout and downstream-consumer handoff. It
freezes the application HTTP inventory, attachment proof matrix, immediate
voice default, opaque callback authorization/race rules, shared EN/RU labels,
history/presence/DND/block projection, mixed-version whole downgrade,
coordinator-first bounded exposure, privacy-safe operations signals and
drain-first rollback. The obsolete pre-implementation decomposition and all
three story diagrams now show accepted ownership and runtime/rollout states;
an executable documentation guard keeps the critical exclusions and protocol
entry point from silently drifting. Coordinator vet/full/presentation-race,
pinned previous-head compatibility, Windows vet/tests/amd64+arm64 builds,
Swift release build, all PlantUML renders, diff and board validation passed.
Exact engineering head `14d3d5a6f99a614f4886d42246fd33a61a51459d`
passed all four hosted jobs in run `29374582024`; the best-effort engineering
scope and `STORY-260712-34kbkn` are accepted. No real Telegram client, app,
audible playback, packaged-device or physical-hardware result is claimed;
those remain in `EPIC-260714-th54l3`. Progress is 50/205 overall and 50/186
engineering. Tracking head `c137399a59d83fe58e222191cb4eba57d4d4db28`
passed all four hosted jobs in run `29374771223`; PR #49 landed at merge
`e10762bf6766bc4249d2ab6bedf46c256abe496a`. Strict execution started
`TASK-260712-1c04pk` from that synchronized `main` on branch
`task/task-260712-1c04pk-macos-main-window-menubar-shell`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-1c04pk` adds a testable macOS
14 SwiftUI shell without replacing the existing AppKit process lifecycle. A
minimal `NSHostingController` bridge presents a `NavigationSplitView` with
Home, Create, Join, Try locally, History and Settings; the status item and main
menu expose the same primary actions and keyboard shortcuts. One stable
main-actor action object and observable snapshot share paired/reconnecting/
online/degraded, route, now-playing, DND, volume, recording and history state.
Complete EN/RU copy and text-plus-symbol semantics keep unpaired, degraded and
recording states non-color and VoiceOver-readable. Capture, local self-test and
HTTP history/presence remain visible but honestly disabled seams for their
strict later tasks. Coordinator health now requires `welcome` on the current
socket, and local volume crosses the player queue instead of racing runtime
commands. Information architecture, shortcut/failure matrices, deterministic
catalog/state/action tests and the component diagram document the handoff.
Coordinator full/vet/race, Windows native full/vet/race plus amd64/arm64 cross-
test compilation, Swift release and package builds, PlantUML and board checks
passed locally. Exact engineering head
`895eddfdbab91c3e4cbdf1918136a704277627dd` passed all four hosted jobs in run
`29375974503`. No real app, live VoiceOver, audible output, microphone,
packaged-device, signing/notarization or physical-hardware result is claimed;
those remain in `EPIC-260714-th54l3`. Progress is 51/205 overall and 51/186
engineering. Tracking head `7cea8f824cc4cac7308f93119f12b55b6931a5a6`
passed all four hosted jobs in run `29376248901`; PR #50 landed at merge
`0008147512fdd4d82d9acc7b40a6a61174e490f8`. Strict execution started
`TASK-260712-2lrpc0` from that synchronized `main` on branch
`task/task-260712-2lrpc0-builtin-cue-temp-media-contract`.

Checkpoint 2026-07-15: `TASK-260712-2lrpc0` is accepted at exact engineering
head `52fa9ea40bb400ee5e5e7ca77eb0769eea04f9fc`; all four hosted jobs passed in
run `29377961960`, including 180 Swift tests and the signed MSIX package/install
contract. The repository now has one deterministically generated Relux
Works-owned PCM cue with exact provenance/hash and guarded macOS/Windows package
paths; a shared 17-transition Swift/Go lifecycle; owner-only opaque partial,
self-test and durable-draft storage; picker-copy/token closure; fsync plus atomic
rename; crash recovery; path-free errors; and cue sequencing outside committed
microphone samples. Hosted CI exposed and the final head fixed macOS `/var` to
`/private/var` firmlink identity and directory-mode edge cases. No real
microphone, audible cue, real-app, packaged-device or physical-hardware evidence
is claimed; those remain in `EPIC-260714-th54l3`. Progress is 52/205 overall
and 52/186 engineering. PR #51 tracking and merge remain before strict execution
starts `TASK-260712-30abcm`.

Tracking head `9391e5e3dc73b9fe07a0600b89681e2b3fd971f7` passed all four
hosted jobs in run `29378145671`; PR #51 landed at merge
`1a7d68cd9e8ef2b2ff4b1809bda43757f5c97774`. Strict execution started
`TASK-260712-30abcm` from that synchronized `main` on branch
`task/task-260712-30abcm-macos-microphone-capture-engine`.

Checkpoint 2026-07-15: `TASK-260712-30abcm` is accepted at exact engineering
head `18bae352eb76544dd13b2aa0bf646c887926c43b`; all four hosted jobs passed in
run `29379013937`, including 188 Swift tests. The repository now has an
explicit Record-only TCC boundary, default/selected CoreAudio input capture,
mono 48 kHz PCM16 app-private drafts, a local bounded meter, exact 180-second
and 50-MiB caps, generation-safe TCC cancellation, start/stop cue exclusion,
-12 dB main-program ducking and one serialized cleanup owner for device, TCC,
sleep, session, quit and backend terminals. The packaged app carries guarded
English/Russian microphone purpose strings. No real microphone, TCC UI,
audible, lifecycle-transition, packaged-device or hardware result is claimed;
those remain in `EPIC-260714-th54l3`. Progress is 53/205 overall and 53/186
engineering. PR #52 tracking and merge remain before strict execution starts
`TASK-260712-9i5se7`.

Tracking head `8ba7de6da04d8353044ce202e155bef0ed842ecb` passed all four hosted
jobs in run `29379179792`; PR #52 landed at merge
`44c4bf3b60d9f4b29cda7639f7f2a1e775356025`. Strict execution started
`TASK-260712-9i5se7` from that synchronized `main` on branch
`task/task-260712-9i5se7-windows-main-window-tray-shell`.

Checkpoint 2026-07-15: `TASK-260712-9i5se7` is accepted at exact engineering
head `90450979463dcf96035cc4278a79f4d528b0778f`; all four hosted jobs passed in
run `29380174085`. The repository now has a shared EN/RU Windows shell model,
an honest unpaired startup, a native Home/Create/Join/Try locally/History/
Settings main window, direct tray Open/Create/Join/Try/record/DND/Quit paths,
textual non-color state semantics, native Tab/Ctrl navigation and PerMonitorV2
font/layout handling with deterministic 100/125/150/200-percent geometry
coverage. The task also repaired the old tray's ignored `TPM_RETURNCMD` result,
which had left visible commands inert. The first hosted run `29380017663`
exposed Windows-target vet violations; the accepted head replaced direct
message-pointer conversion with checked Win32 memory copying. No real packaged
UI, Narrator or physical DPI evidence is claimed; those remain in
`EPIC-260714-th54l3`. Progress is 54/205 overall and 54/186 engineering. PR #53
tracking and merge remain before strict execution starts `TASK-260712-2w4gyw`.

Tracking head `647aafdbb0c8424bc7e6a26c8c06339c47218dfb` passed all four hosted
jobs in run `29380324087`; PR #53 landed at merge
`c52012b84d8c80a0ff8ccbbe445a778f381e65b3`. Strict execution started
`TASK-260712-2w4gyw` from that synchronized `main` on branch
`task/task-260712-2w4gyw-windows-microphone-capture-engine`.

Checkpoint 2026-07-15: `TASK-260712-2w4gyw` is accepted at exact engineering
head `b40bd16c378e2843ebe16ed38cdb8e076de4de43`. The production Windows capture
service now promotes the selected signed-probe AppCapability/WASAPI helper ABI
behind an explicit-Record permission boundary, resolves default or selected
input, normalizes native float frames to private 48 kHz mono PCM16 WAV, emits
only a scalar local RMS meter, ducks/restores the main program and atomically
finalizes exactly one durable unsent draft after a normal or hard-limit stop.
Start cue playback completes before native capture opens; unsafe cancel, quit,
lock, suspend, device-loss, revoke, overflow and backend-failure paths close
ownership and delete partials. The production MSIX declares microphone access
and stages `pulsar-capture.dll`. Local Go race tests, Windows amd64 build/vet
and board validation passed. GitHub Actions run `29381000568` passed all four
jobs on rerun, including native C++ tests and signed MSIX build/install/cleanup;
the first attempt hit an unrelated existing overlay callback timing flake
(96/100) which passed unchanged on rerun. No physical microphone, permission
UI, hidden-capture or real lifecycle result is claimed; those remain in
`EPIC-260714-th54l3`. Progress is 55/205 overall and 55/186 engineering. PR #54
tracking and merge remain before strict execution starts `TASK-260712-3lg0ht`.

Tracking head `708995f7e151ec0bf1518f08f0babfff72b28d65` passed all four hosted
jobs in run `29381291476`; PR #54 landed at merge
`a5351f4cc02d72b280a67e8e8b206a0baee3417b`. Strict execution started
`TASK-260712-3lg0ht` from that synchronized `main` on branch
`task/task-260712-3lg0ht-macos-self-test-file-intake`.

Checkpoint 2026-07-15: `TASK-260712-3lg0ht` is accepted at exact engineering
head `50b872d`. The macOS offline domain flow now plays the reviewed builtin cue
or records exactly five seconds between serialized cues and plays only the
completed draft through the same production clip mixer/output branch. That
local schedule has no coordinator, fetch, upload or mixer-telemetry ownership;
close and explicit delete remove the owned draft. System picker and drag/drop
surfaces show content-probed format, duration, size, audience, eligible modes,
rights and authoritative-server guidance before explicit acceptance. Intake
rechecks security-scoped content, rejects unsupported/unreadable/over-limit
files and streams accepted input to owner-only canonical PCM16 storage. Root
review and local Go, 193 Swift tests, release Swift build, app packaging/cue/
plist/codesign checks passed. GitHub Actions run `29382291652` passed all four
jobs. No real microphone, audible route or Finder observation is claimed;
those remain in `EPIC-260714-th54l3`. Progress is 56/205 overall and 56/186
engineering. PR #55 tracking and merge remain before strict execution starts
`TASK-260712-ut6akw`.

Tracking head `1b45941cd7372f7f311bfb7829f4ae5c111eab5b` passed all four hosted
jobs in run `29382420204`; PR #55 landed at merge
`3f9cbdbf86ca55fb87f1c6933535c517d3a7a516`. Strict execution started
`TASK-260712-ut6akw` from that synchronized `main` on branch
`task/task-260712-ut6akw-macos-hotkey-menubar-recording`.

Checkpoint 2026-07-15: `TASK-260712-ut6akw` is accepted at exact engineering
head `188c30d6bb899a77c23bb415602b99b62b9990f2`. The macOS shortcut controller
uses exclusive `RegisterEventHotKey` registrations for a bounded set of
modifier-bearing Space/R presets and exposes explicit registered, conflict,
unavailable, suspended and inactive states without a global event monitor,
event tap or Accessibility entitlement. Generation fencing makes late callbacks
inert; reconfiguration, sleep, session inactivity, wake, quit and repeated
teardown own at most one hook. Focused Escape stays local to the SwiftUI window,
while an active hidden recording exposes an explicit status-menu Cancel action;
window and menu recording remain independent fallbacks. Validated persistence,
EN/RU projection, API source guards and lifecycle behavior are covered by the
test suite. Local 202 Swift tests, release build, app packaging, cue/plist,
strict codesign and entitlement checks passed. GitHub Actions run `29383052378`
passed all four jobs, including 202 hosted Swift tests, coordinator checks,
Windows cross-build and signed MSIX package/install/cleanup. No physical key,
real conflict, hidden-window, sleep/lock or packaged-sandbox observation is
claimed; those remain in `EPIC-260714-th54l3`. Progress is 57/205 overall and
57/186 engineering. PR #56 tracking and merge remain before strict execution
starts `TASK-260712-25at8b`.

Tracking head `9fb5718e3a38dc418a0237c79a57f6763838f872` passed all four hosted
jobs in run `29383232334`; PR #56 landed at merge
`893125faa25744b08148ea6f72b364e3c823bb77`. Strict execution started
`TASK-260712-25at8b` from that synchronized `main` on branch
`task/task-260712-25at8b-windows-self-test-file-intake`.

Checkpoint 2026-07-15: `TASK-260712-25at8b` is accepted at exact engineering
head `88868cc64f6e7e1059cf7fe759eade71f097cf92`. The Windows offline flow now
uses a direct facade over the production overlay mixer with synthetic local
schedules, mixer reports disabled and no `MediaClipClient`, fetch, coordinator,
upload or receipt ownership. Capture requests can explicitly use the
restart-disposable `self_test` class; the controller sequences reviewed cues,
records for exactly five seconds and plays only the completed self-test draft.
Generation fencing and single-owner cancellation delete stale, replaced,
closed and explicitly deleted artifacts, while capture denial leaves builtin
cue and brokered file review usable. Picker/drop adapters receive a pathless
broker-authorized stream seam; review enforces actual content, 50 MiB/180 s
limits, 60-second overlay eligibility, targets, delivery modes and rights
guidance. Strict RIFF/WAVE PCM16 or float32 input is canonicalized to bounded
private PCM16; other recognized formats are honestly unavailable in the current
local Windows decoder rather than falsely accepted. Production MSIX staging now
includes the exact reviewed cue. Local coordinator tests, 202 Swift tests, Go
vet, full Windows race suite, 82.6% new-flow coverage, Windows amd64 cross-build,
YAML parse and board validation passed. GitHub Actions run `29384112933` passed
all four jobs including signed MSIX package/install/cleanup. No real microphone,
audible route, permission UI, Explorer picker/drop, clean install or packaged
AppContainer observation is claimed; those remain in `EPIC-260714-th54l3`.
Progress is 58/205 overall and 58/186 engineering. PR #57 tracking and merge
remain before strict execution starts `TASK-260712-c7dmv8`.

Tracking head `4b2782565c38fd77aa9254e3ba5f1b32d1a902db` passed all four hosted
jobs in run `29384245685`; PR #57 landed at merge
`0b7ac742b6f7a263f203c7c0ff58489704b1d529`. Strict execution started
`TASK-260712-c7dmv8` from that synchronized `main` on branch
`task/task-260712-c7dmv8-windows-hotkey-tray-recording`.

Checkpoint 2026-07-15: `TASK-260712-c7dmv8` is accepted at exact engineering
head `e70e2ea25ee7a7e335032336b6d962ec9517e230`. The production Windows shell
now has one asynchronous recording controller shared by main-window, tray and
hotkey actions, backed by the existing microphone service, reviewed cues,
production local mixer and private capture store. The tray HWND owns bounded
`RegisterHotKey`/`UnregisterHotKey` registrations for persisted
Ctrl+Shift+Space or Ctrl+Alt+R choices; conflict, unavailable, suspended and
active states remain textual in EN/RU while direct buttons stay independent.
Queued stale IDs are inert. WTS lock/unlock and power suspend/resume maintain a
set of overlapping suspension reasons, cancel capture with exact lifecycle
reasons and never re-register early. `Esc` is a main-window-only accelerator;
hidden recording exposes explicit tray Cancel and there is no low-level or
global bare-Escape hook. Capture start/stop/cancel never blocks the Win32 pump,
and quit performs a bounded post-pump drain before audio teardown. Local Go
tests/vet, full race, repeated focused race, Windows amd64 build/test compile,
coordinator full plus pinned rollback gates, 202 Swift tests under full Xcode,
board validation and diff checks passed. GitHub Actions run `29385014150`
passed all four jobs after rerunning the portable Windows job whose first
attempt failed while downloading Go modules from the proxy; the exact code
head did not change. No physical shortcut, real conflict, tray/Narrator,
microphone, audible cue, lock/suspend or packaged-AppContainer observation is
claimed; those remain in `EPIC-260714-th54l3`. Progress is 59/205 overall and
59/186 engineering. PR #58 tracking and merge remain before strict execution
starts `TASK-260712-1s6h6t`.

Tracking head `c9e0ad0b902f656cbeb48bc4477ab850fb016870` passed all four hosted
jobs in run `29385206888`; PR #58 landed at merge
`707593ecf43c6ad31a9c60676940ff7f8a941e34`. Strict execution started
`TASK-260712-1s6h6t` from that synchronized `main` on branch
`task/task-260712-1s6h6t-macos-local-capture-self-test`.

Checkpoint 2026-07-15: `TASK-260712-1s6h6t` is accepted at exact engineering
head `f8e9db9`. The macOS app now has one composition and operation gate for
the accepted TCC capture engine, exact five-second self-test, file intake,
production local clip output and persisted Carbon shortcut. Accountless mode
starts the same production audio graph without coordinator/librespot and hands
ownership cleanly to the paired runtime. Normal capture serializes reviewed
start/stop cues, publishes a bounded durable draft only after finalization and
stop-cue success, exposes persisted enumerated input selection plus bounded
meters, and projects processing/recording/failure state consistently in the
window, menu and hotkey paths. Self-test and normal recording cannot overlap;
foreground Escape, hidden-menu Cancel, conflict fallback, sleep/session and
quit keep their accepted bounded ownership. Local coordinator vet/full plus
pinned rollback, Windows vet/full/cross-build, 205 Swift tests, release build,
packaged cue/plist/strict codesign verification, board validation and diff
checks passed. GitHub Actions run `29385946438` passed all four jobs, including
signed MSIX build/install/cleanup. No real TCC dialog, microphone, audible
route/cue, Finder, physical shortcut/conflict, sleep/session or signed
production-app observation is claimed; those remain in `EPIC-260714-th54l3`.
Progress is 60/205 overall and 60/186 engineering. PR #59 tracking and merge
remain before strict execution starts `TASK-260712-1p8ykc`.

Tracking head `6fd8dc5db2ce819ae6134f42d34519f894b683ab` passed all four hosted
jobs in run `29386121628`; PR #59 landed at merge
`c0f0509f55fb2162ea67a42ee62f0925c416b55b`. Strict execution started
`TASK-260712-1p8ykc` from that synchronized `main` on branch
`task/task-260712-1p8ykc-windows-local-capture-self-test`.

Checkpoint 2026-07-15: `TASK-260712-1p8ykc` is accepted at exact engineering
head `d29f391`. Paired and accountless Windows launches now construct the same
single-owner local capture workflow. Stable AppCapability input selection,
active `IMMDevice` output switching with a drained WASAPI owner, bounded meter,
reviewed cues, exact five-second record-then-play, normal durable drafts,
broker-handle `FileOpenPicker`, Explorer drop fallback and window/tray/hotkey
actions share one honest state projection. Normal recording and self-test are
mutually exclusive; Escape, lock, suspend, permission revoke and quit cancel,
drain, delete disposable media and only then unsubscribe/close the native
helper. Local Go test/race/vet, Windows amd64 cross-vet/build, coordinator
vet/full and 205 Swift tests passed. All four hosted jobs passed in run
`29387172394`, including native helper tests and reproducible signed MSIX
packaging. No real permission UI, microphone, speaker, audible cue/loopback,
Explorer, shortcut/conflict, lock/suspend or packaged-AppContainer observation
is claimed; those remain in `EPIC-260714-th54l3`. Progress is 61/205 overall
and 61/186 engineering. PR #60 tracking and merge remain before strict
execution starts `TASK-260712-3dqc3l`.

Tracking head `089122c` passed all four hosted jobs in run `29387329098`; PR
#60 landed at merge `22bd461a822e47983f50a696c3165b1720e26f03`. Strict
execution started `TASK-260712-3dqc3l` from that synchronized `main` on branch
`task/task-260712-3dqc3l-macos-ui-data-integration`.

Checkpoint 2026-07-15: `TASK-260712-3dqc3l` is accepted at exact engineering
head `04f4c0f`. The macOS shell now binds self-service Create/Join to protected
identity storage and explicit recovery export, and binds authenticated upload,
transmission, presence, history, receipts and allowed policy actions to the
accepted Phase 1 contracts. Finalized user drafts persist through coordinator
failure and process restart with frozen intent, stable idempotency keys,
confirmed-upload cleanup, transmission-only retry and explicit local/remote
delete. Self-test remains local and disposable. Local verification passed 211
Swift tests in 35 suites, focused PhaseOne and application-boundary tests, and
release build. Exact-head hosted run `29388582864` passed coordinator,
node-core, pulsar-win and pulsar-win-packaged-probe. No real-app, physical
audio, hardware or live-outage observation is claimed; those remain in
`EPIC-260714-th54l3`. Progress is 62/205 overall and 62/186 engineering. PR
#61 tracking and merge remain before strict execution starts
`TASK-260712-2fe5bz`.

Tracking head `90ef80d` passed all four hosted jobs in run `29388719929`; PR
#61 landed at merge `04a23c8`. Strict execution started
`TASK-260712-2fe5bz` from that synchronized `main` on branch
`task/task-260712-2fe5bz-windows-ui-data-integration`.

Checkpoint 2026-07-15: `TASK-260712-2fe5bz` is accepted at exact engineering
head `af961a5`. The Windows shell now binds self-service Create/Join to DPAPI
identity storage and explicit recovery export, plus authenticated upload,
transmission, routing, presence, history receipts and allowed policy actions.
Microphone and explicitly picked-file drafts share a durable owner-only outbox
while retaining their frozen source provenance, route, requested delivery and
stable idempotency keys across outage and restart. Upload confirmation is
persisted before exact local cleanup; retry cannot duplicate upload or
transmission; delete and explicit memory-only interrupt fallback confirmation
remain honest. Self-test is still local/disposable. Local Go test/race/vet,
Windows amd64 cross-vet/build, coordinator test/vet, and 211 Swift tests in 35
suites passed. Exact-head hosted run `29390609436` passed coordinator,
node-core, pulsar-win and signed pulsar-win-packaged-probe. No real Windows UI,
DPAPI prompt, physical audio, live outage or hardware observation is claimed;
those remain in `EPIC-260714-th54l3`. Progress is 63/205 overall and 63/186
engineering. PR #62 tracking and merge remain before strict execution starts
`TASK-260712-1cdoxh`.

Tracking head `05014e0` passed all four hosted jobs in run `29390804757`; PR
#62 landed at merge `5be6f15`. Strict execution started
`TASK-260712-1cdoxh` from that synchronized `main` on branch
`task/task-260712-1cdoxh-acceptance-env-gate-repair`.

Checkpoint 2026-07-15: `TASK-260712-1cdoxh` is accepted at exact replacement
head `f8ae90328bc1448f018bb6c8727c2680bd49e063`. Go 1.25.0, Xcode 26.2 build
17C52, Swift 6.2.3 and explicit GitHub runner images are machine-pinned. A
single mode-0700 runner records sanitized command logs, exact commit/toolchain
provenance and artifact SHA-256; its clean exact-head run passed all 12 stages,
including full/vet, golden and exact predecessor rollback, Windows race and
amd64/arm64 cross-builds, and 211 Swift tests. The fresh two-Pulsar fixture
creates deterministic IDs while keeping runtime credentials in a separate
mode-0600 file. WACK package identity, p95/clock/memory methods, screenshot and
result templates fail closed. Hosted run `29391844793` passed coordinator,
node-core, pulsar-win and signed packaged-probe jobs. No Windows 10/11 real-app,
WACK execution/review, audible, hardware, screenshot or Partner Center result
is claimed; all remain manual under `EPIC-260714-th54l3`. Tracking run
`29392093503` correctly stopped the first merge candidate: it exposed a cancellation race
in the Windows self-test fake, a zero-busy-timeout concurrent SQLite inspector,
and post-suite repository drift that was not yet named in manifests. The task
returned to development. Targeted fixes pass 20 repetitions under race/SQLite
contention, and the harness now fails end-dirty runs with sanitized paths. A
second fail-closed run named Python bytecode as the only hosted drift; disabling
bytecode in the contract subprocess removed it without hiding files. The final
clean local run passed 12/12 with start/end dirty false; hosted run
`29392625265` passed all four jobs, and all three uploaded harness manifests are
`pass` with empty `endDirtyPaths`. Progress is 64/205 overall and 64/186
engineering. PR #63 tracking and merge remain before strict execution starts
`TASK-260712-pbfz37`.

Tracking head `42f9ab2` passed all four hosted jobs in run `29392816247`; PR
#63 landed at merge `f08d16784c4455e46169d0de8292686c419ff745`. Strict
execution started `TASK-260712-pbfz37` from that synchronized `main` on branch
`task/task-260712-pbfz37-windows-report-block-delete`.

Engineering candidate `c6b2819` implements the Windows Phase 1 UGC surface:
the authenticated History view now projects only coordinator-authorized report,
block, owner-delete and replay controls; reports use the six frozen moderation
reasons plus optional bounded details; repeated backend outcomes, denial and
offline failures are localized without exposing opaque IDs. Standard Win32
tab-stop buttons and the explicitly labelled details edit preserve the
keyboard/accessibility contract. `go test -race ./...`, Windows amd64 test
cross-compilation, amd64/arm64 builds and the seven-stage local Windows
acceptance suite pass. The physical packaged-app keyboard/screen-reader pass is
not claimed and remains `TASK-260712-e5mfqj` in `EPIC-260714-th54l3`. Clean
exact-head run `engineering-d5a40c0` passed 7/7 with start/end dirty false and
hosted run `29393834216` passed coordinator, node-core, pulsar-win and the
signed packaged probe. The engineering task is accepted at 65/205 overall and
65/186 engineering. Tracking head `1aeae22` passed all four hosted jobs in run
`29394010912`; PR #64 landed at merge
`ab0992321fee609f984969724415eeaa0629139f`. Strict execution then started
`TASK-260712-34stvx` from synchronized `main` on branch
`task/task-260712-34stvx-macos-report-block-delete`.

Engineering candidate for `TASK-260712-34stvx` adds the canonical macOS
History report, block, owner-delete and replay surface. SwiftUI renders only
coordinator-authorized actions, rechecks current authorization in composition,
uses the six frozen report reasons with optional 2,000-byte details, confirms
destructive/blocking actions, and maps exact success/reuse, denial and offline
codes to privacy-safe EN/RU copy. NodeCore validates canonical request and
response bodies fail-closed. Full local Xcode tests pass 215 tests in 35 suites,
the release build passes, board/diff checks pass, and repository automated Swift
acceptance is green. Physical keyboard and VoiceOver observation is not claimed
and remains manual in `TASK-260712-e5mfqj` under `EPIC-260714-th54l3`.
The clean exact-head acceptance manifest pins engineering commit
`074e5a75826433778014af80487b779d19dec69c` with start/end dirty false and
215 passing tests. Hosted run `29395040109` passed coordinator rollback,
authoritative Xcode Swift, Windows portable/cross-build and the signed packaged
probe. Engineering progress is now 66/205 overall and 66/186 engineering; PR
#65 tracking and merge remain before the Telegram task starts.

Tracking head `34277a4` passed all four hosted jobs in run `29395210052`; PR
#65 landed at merge `1c45953efe2e8b5b4f4112054857bab8552b6a32`. Strict
execution started `TASK-260712-dlltnr` from that synchronized `main` on branch
`task/task-260712-dlltnr-telegram-moderation-parity`.

Engineering candidate for `TASK-260712-dlltnr` adds the private Telegram
`/history` surface for canonical replay, owner delete, six-reason report and
primary sender-block actions without a second moderation implementation.
Opaque 15-minute callbacks bind actor, orbit, role, chat, message, history item,
action and reason; query replay, expiry, forged/cross-user/group attempts and
terminal keyboard removal are covered. Verified Telegram history now projects
current Pulsar receipts in its own Barycenter, while reports retain distinct
reporter and exact installation-evidence identities. The old moderation target
constraint migrates transactionally with rollback, foreign-key and immutable
evidence checks. Engineering commit `8ce1b8cf4ced1840a555c8356b45754e405d21df`
passed the clean exact-head 12-stage repository acceptance suite with
start/end dirty false, plus coordinator vet/full tests and focused race. Live
Bot API, real-account and real-device observations remain unclaimed in
`EPIC-260714-th54l3`. Hosted run `29397089442` passed coordinator rollback,
authoritative Xcode Swift, Windows portable/race/cross-build and the signed
packaged probe on tracking head `88de480`. The engineering scope is accepted;
progress is now 67/205 overall and 67/186 engineering. Final tracking head
`404637f` passed all four hosted jobs in run `29397331332`; PR #66 landed at
merge `4bc841834e8fd9072862ef5d4d2116a4483186ed`. Strict execution then started
`TASK-260712-e1ie4x` from synchronized `main` on branch
`task/task-260712-e1ie4x-platform-declarations-localized-copy`.

Engineering acceptance for `TASK-260712-e1ie4x` freezes one canonical EN/RU
platform-copy contract across both desktop shells, production MSIX resources
and localized macOS privacy strings. Windows keeps the exact reviewed network
plus microphone capability set with no `runFullTrust` or broad filesystem
access; macOS adds no unproven sandbox or Accessibility entitlement. Legacy
Spotify and Telegram paths are plainly optional, and pairing no longer opens a
Spotify completion gate. Exact engineering head
`918b377275db7d01c6646d2fbe8428ec8d4382eb` passed the clean 12-stage suite
with start/end dirty false. Hosted run `29398604558` passed all four jobs; its
Windows SDK job compiled EN/RU PRI resources and packed the production MSIX
schema before completing the existing signed-probe install checks. Actual WACK
UI, installed permission prompts and physical hardware remain unclaimed in
`EPIC-260714-th54l3`. Progress is 68/205 overall and 68/186 engineering; PR
#67 tracking and merge remain before the independent protocol review starts.

Tracking head `0b5f0ad` passed all four hosted jobs in run `29398828754`; PR
#67 landed at merge `aa86926103688473cc1d99185f627e277095f5a0`. Strict
execution started `TASK-260712-176b74` from synchronized `main` on branch
`task/task-260712-176b74-p1-independent-protocol-review`. The technical audit
will execute inline under the requested no-subagent rule. Because this same
execution chain authored some reviewed protocol and scheduler changes, the
distinct non-implementing-reviewer criterion is not silently claimed: technical
self-audit and any fixes may complete, but independent signoff remains open
unless a genuinely separate reviewer is authorized.

The self-audit found `P1-PROTO-001` (HIGH): Swift rejected a mismatched envelope
major while coordinator and Windows Go runtime paths accepted it. The corrective
patch now rejects the frame before payload dispatch on both mirrored Go codecs,
rejects pre-auth registration before credential lookup, closes established
coordinator sockets, reconnects Windows and makes the Swift runtime reconnect
decision explicit. Focused tests, coordinator and Windows race suites, 35 Swift
protocol/clip/clock tests and exact predecessor transmission rollback pass.
The reproducible review packet is
`docs/analysis/p1-independent-protocol-technical-audit.md`. No other critical or
high technical finding remains, but the task is not accepted: the explicitly
required non-implementing reviewer signoff is still open.

Corrective engineering head `cde0aa4b2c67157bc06add3e46495e48711ba427`
passed the clean exact-head 12-stage repository acceptance suite with
start/end dirty false. Hosted run `29399875529` passed coordinator, node-core,
pulsar-win and the packaged probe; PR #68 landed the HIGH fix at merge
`524eb78`. The owner's standing goal says external approvals must be accumulated
without stopping reversible engineering, so the genuinely non-implementing
signoff moved to owner-decision `TASK-260715-3ffm3r`. The original task remains
`to-review`, its independence checklist stays honestly open and it does not yet
increase accepted-task counts. Its best-effort engineering scope is exhausted,
so strict execution may now start `TASK-260712-1uz0za`; Phase 1 root acceptance
and Store submission remain withheld until the external signoff returns.

Tracking head `7c60183` passed all four hosted jobs in run `29400207186`; PR
#69 landed the external-signoff ledger at merge
`aed5d7e5225aca0d4d5b0ad8347cfd500f6c0dac`. Strict execution started
`TASK-260712-1uz0za` from synchronized `main`. Its engineering review is limited
to deterministic render ownership, callback safety, concurrency, lifecycle and
failure parity. Real A3/A4 listening, physical timing, packaged applications
and hardware remain exclusively in manual `TASK-260712-2hodti`; a separate
non-implementing audio-reviewer signature will be accumulated in the external
owner ledger instead of being self-claimed.

The realtime-audio audit closed three HIGH findings. Windows now reports
asynchronous mixer/resume failure as typed failure rather than false completion,
honors `resume_main=false`, and consumes one cached interrupt-finalizer outcome
across cancel/natural-end races. macOS now uses atomic FIFO-reader ownership,
serializes multiple gain-command producers outside the render callback, bounds
idle FIFO shutdown and builds heartbeat state as one queue-owned snapshot.
Exact engineering head `805337d0d572f6e45b90fc76120af29f21be89e3`
passed the clean 12-stage repository acceptance suite with start/end dirty
false; the complete Swift suite passed 218 tests and Windows passed its full
race suite. Hosted run `29401627207` passed all four jobs, and PR #70 landed at
merge `5aedd6817bece741b76408135271a5fb8da40a83`. Independent reviewer plus
manual A3/A4 completion is routed to owner task `TASK-260715-s838ym` and
hardware task `TASK-260712-2hodti`. The original review remains `to-review` and
does not increase accepted-task counts; its engineering scope is exhausted, so
strict execution advances to `TASK-260712-1xkn75`.

Tracking head `6d62cf0` passed all four hosted jobs in run `29401906752`; PR
#71 landed the external audio-signoff ledger at merge
`635a8d3e3e9d7929a474ae6a5278187071c520c9`. Strict execution started
`TASK-260712-1xkn75` from synchronized `main` on branch
`task/task-260712-1xkn75-p1-independent-migration-review`.

The migration audit closed two HIGH findings. Legacy and orbit bootstrap now
installs atomically and no longer discards `media.orbit_id`, `slots.provider`
or `members.display_name` migration errors. Connection setup installs and
retries the bounded busy policy before WAL negotiation, so concurrent rollout
startup serializes instead of failing before `busy_timeout`. Failure, partial,
ten-run concurrent, full/race and all ten exact-predecessor scenarios pass.
Exact engineering head `7736b7546b7ef86347d86dfefb095c9f795ad9ff`
passed clean 12-stage acceptance with start/end dirty false; hosted run
`29402957156` passed all four jobs. PR #72 landed at merge `d7e0065`.
Independent signoff is routed to owner task `TASK-260715-unbb7c`; the original
review remains `to-review` and does not increase accepted counts. Strict
engineering advances to `TASK-260712-wy05n6`.

The security audit closed three HIGH findings. Legacy pairing now accepts
forwarded source identity only from the loopback TLS terminator and bounds both
source-key and rejected-attempt state. The coordinator now bounds public HTTP
transport and unauthenticated WebSocket registrations. Both Go modules and the
acceptance authority now select patched Go 1.25.12; exact vulnerability scans
report no reachable advisories. Exact engineering head
`a87532c745195fe6772dd03882d2154364509b8b` passed clean 12-stage acceptance
with start/end dirty false, both Go race suites and all 218 Swift tests. Hosted
run `29404910264` passed all four jobs; PR #74 landed at merge
`dab3999c34dc8844eae5202dc72c1baf71ce8507`. Independent signoff is routed to
owner task `TASK-260715-10ksxz`; the original review remains `to-review` and
does not increase accepted counts. Strict engineering advances to
`TASK-260712-2s4e9p`.

Tracking head `866d000` passed all four hosted jobs in run `29405188885`; PR
#75 landed the external security-signoff ledger at merge `6664ffd`. Strict
execution started `TASK-260712-2s4e9p` from synchronized `main`. Repository
work covers exact versioned EN/RU listing inputs, IARC/certification answers,
asset manifests and validation. Actual localized screenshots, WACK execution
and Partner Center mutation remain manual/external evidence and cannot be
self-claimed from this engineering environment.

The Store engineering package replaces the historical Spotify-first draft
with exact EN/RU self-contained Phase 1 inputs, binds approved locale links and
shipped-claim evidence, records a source-linked IARC truth profile, directly
answers `10.3.1` and `10.1.1.3`, and reserves six real screenshot slots per
locale. Its validator checks Partner Center field limits, path containment,
PNG dimensions/hashes and every exact-build/manual/owner gate; default
engineering validation passes while `--require-ready` fails closed. Live
policy pages match approved hashes. Exact engineering head
`99f195704d8ef66b6c8f324e023f0504a3e1bc1c` passed clean 12-stage acceptance;
hosted run `29406679102` passed all four jobs. PR #76 landed at merge
`ee0cf0313487cd5bb54208d41f1d1bbce783d8c4`. Real screenshots and WACK remain
in existing manual task `TASK-260712-e5mfqj`; exact IARC, candidate hashes and
owner proceed remain in `TASK-260715-24ube9`. The original task stays
`to-review`, does not increase accepted counts, and strict engineering advances
to `TASK-260712-38lssj`.

The Phase 1 root integration review accepts exact engineering candidate
`16420c2ce652d05d534fb45b5ef9a7124d4bbdd6` for reversible P2 coding while
explicitly withholding product, Store and release acceptance. Its deterministic
manifest covers all 68 first-parent intervals and 737 no-renames path entries
from approved baseline `38ebd385`, embeds every task AC, maps every path to
A1-A8 and has zero unmapped files. Root semantic review rechecked all nine
closed HIGH findings across protocol, realtime audio, migration and security;
no critical or high engineering finding remains. Coordinator and Windows full
race suites pass, as do 218 Swift tests in 35 suites. Exact review-packet head
`4c79d12bb2982f6916d8e612b7d6d50a3732ee2f` passed clean local acceptance
12/12 with start/end dirty false. All four PR jobs passed in run `29408109562`;
the Actions API binds the run to head `4c79d12`, while its downloaded manifests
honestly record checked-out synthetic merge-ref `cfcfc0c`.
PR #78 landed at merge `0762ed232492c95829f152922a0e5d1ab3a5c397`.
Independent signatures, physical A1-A8, WACK/screenshots, IARC/Partner Center
and mailbox delivery remain fail-closed in their external/manual ledgers; they
no longer block the explicitly engineering-only readiness handoff. Progress is
69/205 overall and 69/186 engineering, and strict execution advances to
`TASK-260712-1xik11`.

The P1 engineering readiness handoff freezes exact source candidate `16420c2`,
root packet `4c79d12`, local acceptance and four hosted artifact IDs in a
fail-closed machine authority. Every A1-A8 scenario keeps deterministic
engineering coverage and `manual-required`, mapped in strict order to
`TASK-260712-1vtwkl`, `TASK-260712-2hodti` and `TASK-260712-e5mfqj`; the latter
now explicitly includes the previously missing real Telegram A5 FIFO/callback
check. The test-signed AppContainer probe is plainly not a production
candidate. Release, Store submission and Partner Center mutation remain false;
six external owner holds remain open. Legal inputs, exact policy hashes, live
public pages, default Store/listing and moderation gates pass; production Store
readiness and mailbox-ready gates fail exactly as expected. Both Go 1.25.12
vulnerability scans report no vulnerabilities. Initial hosted run
`29409214595` exposed that three CI jobs used depth-1 checkout and could not
verify historical commit-to-tree provenance; fix head
`ce33d1724d46a6b691f9a088cc9da9dc9d28b6c6` makes all four checkouts full-depth
and adds a regression assertion. That head passed clean local acceptance 12/12
and hosted run `29409373973` passed all four jobs. PR #80 landed at merge
`9bf3d100cf69388ba76cc40bddd3906e91e72f26`. Progress is 70/205 overall and
70/186 engineering. P1 engineering is complete, product/release acceptance is
not claimed, and strict execution advances to `TASK-260712-17yizc`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-17yizc` freezes the normative
Phase 2 Air lifecycle and policy contract on exact engineering head
`77fb68231e0c18a1ecb9bdeae5725386d5e64a1a`. The reviewed contract separates
saved membership from the single active-Air pointer, requires joining-primary
confirmation, defines secure one-time invites, explicit create/read/activate/
deactivate/leave/ownership/policy/dissolve routes, exact Telegram aliases,
parked and join/leave audio behavior, immutable accepted target/policy
snapshots, capacity and audit/error vocabularies. A persisted authority
generation prohibits simultaneous link and Air runtimes and fails unsafe
rollback closed. The executable JSON summary and Go document guards freeze
statuses, roles, 15 routes, limits, defaults and invariants. Self-review found
and closed an alias activation gap before publication. Clean local acceptance
passed 12/12; hosted run `29410722718` passed all four jobs. PR #82 landed at
merge `b5d10b26b22fc4cae88fef590191f8015f401fb9`. Progress is 71/205 overall
and 71/186 engineering; strict execution advances to `TASK-260712-3n36ny`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-3n36ny` adds the complete
additive Air persistence foundation on exact engineering head
`b5a633932e7d616bbdee252e1f255c2dfbf49054`. Airs, saved/pending/left
memberships, invite hashes, revisioned policies, audits and per-barycenter
active pointers are transactional and enforce one active Air. Legacy active
links backfill exactly once into deterministic two-member Airs. Persisted
authority generations and immutable legacy runtime snapshots make cutover and
rollback single-authority and fail closed on unsafe old-binary writes. Failure
injection covers DDL/backfill/cutover rollback, concurrent lifecycle updates
have one winner, and the exact predecessor coordinator creates/breaks legacy
links while preserving unknown Phase 2 rows. Self-review also fixed a
concurrent identity-bootstrap ALTER race and separated legitimate Air restart
state from legacy mutation detection. Clean local acceptance passed 12/12;
hosted run `29413065743` passed all four jobs. PR #84 landed at merge
`68059d9c03d6af3dcdd84468805309d4be559901`. Progress is 72/205 overall and
72/186 engineering; strict execution advances to `TASK-260712-kr64r2`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-kr64r2` makes stable public Air
ID the only Phase 2 shared-runtime and persisted-session owner on exact
engineering head `d344f32e20bf1934022acdefc241fbc34a8c0ff9`. Runtime resolution
uses only joined members with a current pointer to the same active Air, so
saved memberships cannot create transitive peer unions. Startup warms only
active two-or-more-member Airs; parked/saved rooms remain lazy. Authority
generation and Air revision fence timers, Telegram ordering and all background
playlist/metadata/provider completions. Joiners catch up only the current main
track, leavers stop locally while remaining members keep the same FSM, and
media lifecycle cancellation persists Air snapshots. Restart, direct Air
switch, rollback-hold, stale completion and exact fanout regressions are
covered. Clean local acceptance passed 12/12 with a clean tree; hosted run
`29415681872` passed all four jobs after one retry of an unrelated pre-existing
Windows callback-dispatch scheduling flake. PR #86 landed at merge
`3dcf309f623c55a8d3bfa6f4582b2c194cc96d7c`. Progress is 73/205 overall and
73/186 engineering; strict execution advances to `TASK-260712-2vhf80`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-2vhf80` exposes all 15 frozen
Air lifecycle routes through transactional control-token `ActorContext` on
exact engineering head `efa02ac`. Create/list/read, secure invite issue and
consume, joining-primary confirm/decline, activate/deactivate/switch, leave,
role and ownership governance, policy replacement and dissolve use strict
JSON, opaque IDs, stable errors and actor-scoped exact idempotency. Invite
codes are 256-bit, fixed-TTL and single-use; only a keyed HMAC reaches SQLite,
while exact retries deterministically reproduce the one-time response.
Concurrent consume, eight-barycenter capacity, foreign-room collapse, wrong
confirmer, governance, restart persistence, audit and secret-redaction paths
are covered. Runtime-changing HTTP success now waits for the serialized Air
resolver to apply the committed authority generation and park stale sessions.
Full Go tests, vet and targeted race passed; clean pinned coordinator
acceptance passed 5/5. Hosted run `29418360729` passed all four jobs, and PR
#88 landed at merge `69f32e2a062709bfff2058cc9e39f4d6932ee391`.
Progress is 74/205 overall and 74/186 engineering; strict execution advances
to `TASK-260712-25862f`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-25862f` enforces the frozen
invite/overlay/queue/replace policy across transactional generic media,
Telegram compatibility commands and app-originated external playback on exact
engineering head `7a3e31f`. Accepted transmissions now persist immutable Air
ID, policy revision, operation and authorization result snapshots; active-Air
target expansion uses only current joined pointers, and later membership or
policy changes cannot reauthorize or expand the target ACL. Local block/DND,
capability, binding and media ownership remain stronger. Migrated pairwise Airs
retain their legacy numeric FIFO domain while new Airs use a stable internal
domain behind the opaque Air ID. Complete policy old/new JSON is versioned and
audited. Full Go tests, vet, targeted race and exact previous-head rollback
passed; hosted run `29420598338` passed all four jobs, and PR #90 landed at
merge `aa40b50a0157d1482957a7550b6a9035e26cdc5b`. Progress is 75/205 overall
and 75/186 engineering; strict execution advances to `TASK-260712-2bjdlb`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-2bjdlb` makes the Air model the
only `/approach`, `/accept`, `/decline` and `/apart` authority after cutover on
exact engineering head `d2af5aa`. Parked Air, owner membership, creator
pointer and hashed one-time invite are created atomically; retry derives the
same plaintext-free compatibility code. Claim never activates, only the
joining primary confirms, and another current Air cannot be switched
silently. Decline/cancel/withdraw tombstone abandoned aliases; `/apart`
removes only the caller and transfers ownership when needed. Existing donor
handoff and personal session parking feed the stable Air runtime, while home,
status and notifications expose only human titles. Duplicate delivery,
migration restart and rollback-hold coverage prove that neither the alias nor
the frozen legacy link can create a second or resurrected runtime. Full Go
tests, vet, targeted race and exact previous-head Air rollback passed locally;
hosted run `29422446508` passed all four jobs, and PR #92 landed at merge
`095bf823f9a8a2665fb7e4d363b6abc0e5def166`. No real-app or physical-hardware
result is claimed. Progress is 76/205 overall and 76/186 engineering; strict
execution advances to `TASK-260712-2i3u7v`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-2i3u7v` adds native macOS Air
room management on engineering commit `13a65d1`. The authenticated common Air
client and observable composition expose saved and current Airs, pending
joining-primary confirmation, aggregate membership/capacity and effective
policies. Create, invite, consume, confirm, decline, activate, switch,
deactivate, leave, dissolve and policy actions use stable server errors;
disruptive transitions require item-bound confirmation. Invite secrets remain
memory-only, are redacted from descriptions and accessibility labels, and the
clipboard is conditionally cleared. EN/RU keyboard and VoiceOver paths are
covered without exposing raw IDs or coupling to Phase 1 target/inbox models.
Local Xcode Swift tests passed 221/221 and `swift build` passed; hosted run
`29424982574` passed all four jobs. PR #94 landed at merge
`8cd46b1be322ec67f0f102046eb8c134939cbe18`. No real-app or physical-hardware
result is claimed. Progress is 77/205 overall and 77/186 engineering; strict
execution advances to `TASK-260712-31zja2`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-31zja2` adds native Windows Air
management on engineering commit `8b458d8`. A strict authenticated common API
client and independently polled composition expose saved/current Airs,
joining-primary preview, aggregate membership/capacity, role and all effective
policies. Create, invite, consume, confirm/decline, activate/switch/deactivate,
leave, dissolve and complete policy presets use exact revisions and stable
errors. Switch, deactivate, leave, dissolve and join-with-switch require a
separate confirmation command. Invite input is password-style; issued secrets
remain memory-only, are absent from window/accessibility text and formatting,
expire in memory, and use history/cloud-excluded compare-and-clear clipboard
leases. EN/RU native-control, Ctrl+3, screen-reader source and DPI-scaled layout
seams are covered without raw IDs or Phase 1 target/inbox coupling. The exact
Windows automated suite passed 7/7 and the repository suite passed 12/12;
hosted run `29428413069` passed all four jobs. PR #96 landed at merge
`203bb1e2eddf50002f1d73d0f146557c033745c3`. One pre-existing websocket
teardown flake occurred once locally; the uncached retry passed and the exact
test passed 10/10. No real-app, physical-hardware, live screen-reader or live
high-DPI result is claimed. Progress is 78/205 overall and 78/186 engineering;
strict execution advances to `TASK-260712-2zdetx`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-2zdetx` exposes the canonical
Air lifecycle through a private Telegram `/air` surface on engineering commit
`e8d8214`. Create, list, invite, single-use consume, joining-primary
confirmation/decline, activate/switch/park, leave, dissolve, withdrawal and
owner policy presets call the same transactional Air store as Pulsar; existing
approach/apart commands remain compatibility aliases over that store and the
single Air runtime reconciler. Durable opaque callbacks bind freshly resolved
actor, orbit, role, chat, message and lifecycle revisions, expire after 15
minutes and fence Telegram query replay. Invite secrets stay out of callback
data, inline prompt text, logs and durable mutation results; successful consume
best-effort deletes the secret-bearing source message. Full coordinator
test/vet/race passed, focused security and E2E tests passed 10/10 plus targeted
race, and the repository automated gate passed 12/12 including exact
previous-head rollback. Hosted run `29430796136` passed all four jobs. PR #98
landed at merge `009fba2e9e3f93bd36614725da3702c76625ba1f`. No real Telegram
client, app or hardware evidence is claimed. Progress is 79/205 overall and
79/186 engineering; strict execution advances to `TASK-260712-3nq0tq`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-3nq0tq` closes the Air story
with an adversarial lifecycle and capacity rehearsal on engineering commit
`b984230`. The generic transmission scheduler now rechecks that every accepted
Air target is still a joined member with that Air active: a leaver is cancelled
during prepare or moved to cancelling during playback with stable
`approach_left`, while remaining targets continue and the immutable accepted
snapshot is never expanded. A deterministic production-loop fixture proves 8
Barycenters, 20 Pulsars, 20 unique load commands, zero duplicates, one Air
runtime and zero legacy groups. The repository-only harness covers one-active
and role/invite races, exact current union, join/leave boundaries, lazy parking,
restart, aliases, secure Telegram callbacks, fault-injected backfill/cutover,
rollback hold and explicit downstream gaps. Full coordinator tests/vet/race
passed; repository acceptance passed 12/12 including exact predecessor,
Windows race/cross-build and Swift fixtures. Hosted run `29432415158` passed
all four jobs; PR #100 landed at merge
`e4aa266913ed7daed1ae07c50d7b33c1e7d1288f`. Real applications, hardware,
Telegram transport and audible playback remain unclaimed in the manual-test
epic. Progress is 80/205 overall and 80/186 engineering; the Air story is
complete and strict execution advances to `TASK-260712-14u0yk`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-14u0yk` freezes the
candidate-neutral codec/player spike contract on engineering commit `aba592e`.
The versioned rubric defines identical MP3, AAC-LC and Opus fixture recipes,
long-duration CBR/VBR and hostile cases, authenticated single-range/cache and
fault semantics, exact warm-up/sample counts, artifact schemas and immutable
hard gates for start, seek, cross-node skew, RSS and duration-independent
memory. The content-addressed generator requires exact FFmpeg 8.1.2 source and
signature inputs; the bounded streaming harness rejects unsafe or mismatched
fixture locks and does not expose bearer tokens. A fail-closed evaluator
requires all three platform pairings and distinguishes synthetic
`engineering-pass` from final packaged-hardware evidence. The shortlist pins
native AAC, exact pure-Go modules and bundled FFmpeg 8.1.2 without claiming a
decoder, licensing, Store, audible or hardware result. Codec tests passed
10/10 repeatedly and repository acceptance passed 12/12. Hosted run
`29434417154` passed all four jobs; PR #102 landed at merge
`8f91187d3ab9bb62fe31a00407a1a7058df27d9b`. Progress is 81/205 overall and
81/186 engineering; strict execution advances to `TASK-260712-dqdoqj`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-dqdoqj` freezes
`p2-stream-variants-range-cache.v1` on engineering commit `733b5c6`. The
candidate-neutral prototype materializes constrained SQLite `stream_variants`
rows for the exact fixture lock, content-addressed whole/chunk SHA-256
manifests and monotonic chunk-aligned seek maps no more than 10 seconds apart.
The range client and harness require bearer plus immutable target binding on
every request, enforce single-range 206/304/416, strong ETag/If-Range, exact
length/type/digest, `private, no-store`, tenant Vary and uniform ACL denial.
The app-private reference cache freezes 512 MiB global, 64 MiB per variant,
128 MiB pinned and 1 MiB chunk/read ceilings; it uses HMAC namespaces, atomic
LRU, restart reconciliation, concurrent pins, corruption/path/symlink defense
and a durable no-refill ledger for delete or actor disable. Codec/stream tests
passed 8/8 repeatedly and repository acceptance passed 12/12. Hosted run
`29436698927` passed all four jobs; PR #104 landed at merge
`f6dd5c2`. No decoder, license, production ingest, Store, audible or real-
hardware result is claimed. Progress is 82/205 overall and 82/186 engineering;
strict execution advances to `TASK-260712-1vdlkw`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-1vdlkw` freezes the exact
codec license, patent, supply-chain and distribution audit on engineering
commit `3fc2409`. Seven exact components and all three frozen candidates carry
source/version, commit, Go sums, source and license hashes, runtime-transitive,
notice, vulnerability, packaging and legal dispositions dated 2026-07-15.
`pure-go-composite-v1` is rejected: its exact AAC module is GPL-2.0-only,
identifies itself as a FAAD2 port, and its origin was unavailable at audit
time. Native OS codecs and the minimal shared FFmpeg 8.1.2 candidate are only
`shippable-with-obligations`: AAC counsel approval, exact corresponding source
and notices, complete runtime SBOM, zero known unpatched findings, immutable
signed package members, macOS notarization, retained sandbox and no runtime
code download. A fail-closed validator and tamper test enforce the matrix;
codec tests passed 9/9 and repository acceptance passed 12/12. Hosted run
`29437923424` attempt 1 was cancelled after an isolated packaged-runner hang;
attempt 2 passed all four jobs in 1m47s, 1m47s, 2m19s and 2m41s. PR #106
landed at merge `594495b`. No legal advice, patent clearance, decoder, Store,
audible or hardware result is claimed. Progress is 83/205 overall and 83/186
engineering; 122 overall and 103 engineering tasks remain. Strict execution
advances to `TASK-260712-1canzv`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-1canzv` proves the exact
bundled FFmpeg 8.1.2 engineering path on code head `ad51481`, merged by PR
#108 as `666220d`. A pinned LGPL-only configure allowlist, six synthetic
MP3/AAC/Opus CBR/VBR/container fixtures, narrow C bridge and private bounded
cache harness cover decode, scheduled start, pause/cancel, seek generation,
resume, drain, hostile inputs, disk, CPU and RSS. Dedicated hosted run
`29444807851` passed macOS ARM64, macOS Intel and Windows amd64. ARM64 package
size was 2,100,847 bytes with 5,210,112 peak RSS and 93 ms aggregate decode
CPU. The 1,965,989-byte Windows AppContainer MSIX has SHA-256
`c003ceab37a35b21e9bfc8bea168eed735fbf5c0b964355c0c59a860d15bcb50`;
all PE imports and embedded signers are inventoried, including package-local
winpthreads `14.0.0.r190.g96fb1bff7-1`; temporary machine trust supports an
offline installed-package decode and is removed with the package. Windows
peak RSS was 7,028,736 bytes and aggregate decode CPU was 124 ms. Standard CI
run `29444811403` passed all four jobs. Shipping remains rejected pending
Windows ARM64, production signing/notarization, current SBOM/advisory/counsel
review and accepted hostile-input isolation; no Store, production-signing or
physical-hardware result is claimed. Progress is 84/205 overall and 84/186
engineering; strict execution advances to `TASK-260712-298tyq`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-298tyq` proves and rejects the
native Windows Media Foundation candidate on engineering head `4083e5b`,
merged by PR #110 as `6cb817d`. The signed x64 and ARM64 MSIX prototypes retain
the Phase 1 AppContainer posture, declare no capabilities or `runFullTrust`,
activate through `IApplicationActivationManager` with `AO_NONE` and debug
disabled, and self-report an AppContainer token. A bounded range-backed
`IStream` feeds `MFCreateMFByteStreamOnStreamEx` and Media Foundation on one
owned MTA decode thread; scheduled start, pause without reads, generation-safe
seek/resume, drain and cooperative cancellation are automated without network
ownership, WASAPI callbacks or render-thread work. Dedicated hosted run
`29447847569` passed both architectures: all four MP3/AAC fixtures decoded,
while both real Ogg/Opus fixtures produced the exact
`MF_E_UNSUPPORTED_BYTESTREAM_TYPE` value `0xC00D36C4`. The 60,008 ms soak ran
2,214 iterations with 21,970,944-byte start RSS, 24,764,416-byte end RSS,
24,805,376-byte peak RSS and a 262,144-byte maximum underlying read. Local
repository acceptance passed 12/12 and final standard CI run `29448173596`
passed all four jobs. The implementation accepts a requested 7,200-second
soak, but no physical two-hour run or Win10/Win11 hardware matrix is claimed;
those remain in manual epic `EPIC-260714-th54l3`. Shipping is rejected unless
ingest canonicalizes supported input to AAC/M4A and later manual evidence is
accepted. Progress is 85/205 overall and 85/186 engineering; strict execution
advances to `TASK-260712-350u8d`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-350u8d` proves and rejects the
native macOS AVFoundation candidate on native code head `c1ec5c0` and final
engineering head `fb4fe00`, merged by PR #112 as `bbd3f85`. The hardened-
runtime app is ad-hoc signed with exactly the App Sandbox entitlement and no
network entitlement. Its custom `AVAssetResourceLoaderDelegate` is the sole
fixture reader, clamps underlying reads to 65,536 bytes and records complete
range, lifecycle, timing and memory evidence without audio output or a render
callback. Local repository acceptance passed 12/12. Dedicated run
`29449314111` passed hosted ARM64 and x86_64 macOS 15.7.7 jobs: both decoded all
six exact MP3, AAC and Ogg/Opus fixtures; worst seek-to-PCM was 14 ms and 21 ms,
and peak RSS was 28,065,792 and 20,623,360 bytes respectively. Both
architectures nevertheless requested at least the complete source before first
PCM, and cold MP3 CBR scheduled skew was 213 ms ARM64 and 466 ms Intel against
the 100 ms gate. The native candidate is therefore rejected for hidden full-
file preparation and lifecycle timing rather than selected for shipping. Final
standard CI run `29449425341` passed all four jobs. Exact executable, sealed-
resource, entitlement and evidence hashes are attached to the board. Developer
ID signing, notarization, physical hardware, audible routes and supported OS
matrix remain explicitly unclaimed in manual epic `EPIC-260714-th54l3`.
Progress is 86/205 overall and 86/186 engineering; strict execution advances
to `TASK-260712-3vkcki`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-3vkcki` proves and rejects the
exact pure-Go composite on engineering code head `dc1ac49` and documentation
head `7f243e2`, merged by PR #114 as `cbbe39c`. The isolated CGo-free module
graph pins `github.com/hajimehoshi/go-mp3 v0.3.4` and
`github.com/pion/opus v0.1.0`; the audited GPL-2.0-only AAC module is forbidden
and absent from the module graph and binaries. Dedicated hosted run
`29450704499` passed native macOS ARM64, native Windows amd64 and Linux race
jobs. MP3 produced first PCM after 621/237 source bytes, but seek-enabled
construction full-scanned 289,818/52,674 bytes. Ogg/Opus produced first PCM
after 16,264/19,410 bytes and decoded forward, but exposes no random-seek
contract. AAC/M4A and AAC/ADTS are exact zero-read forbidden-module
rejections. The fixed PCM ring peaked at 7,680 of 1,048,576 bytes, underlying
reads remained at or below 636 bytes, hostile truncation/corruption fixtures
did not panic, and eight concurrent Opus decoders passed the race detector.
The macOS binary was 3,568,866 bytes with SHA-256
`b6e81e4fabb847837da8a36c49a8b66648fd7e02aaf1beeac7cf586296a9f74c`;
the Windows binary was 3,746,304 bytes with SHA-256
`5625b40ddbc92e4c8b461b1416e552da64d13651aa5fb938a3b80e09efa1ab79`.
Local repository acceptance passed 12/12 and final standard CI run
`29450856063` passed all four jobs. Heap-system telemetry is not mislabeled
RSS; a two-hour run, AppContainer and physical platform evidence remain
unclaimed in `EPIC-260714-th54l3`. The production candidate is rejected on
license, seek and manual-evidence gates. Progress is 87/205 overall and 87/186
engineering; strict execution advances to `TASK-260712-ibuaxj`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-ibuaxj` publishes a generated,
fail-closed comparison on engineering head `094e96f`, merged by PR #116 as
`10db015`. The contract consumes pinned bundled, Media Foundation, native
macOS and pure-Go artifacts with their repository paths and SHA-256 values,
then expands three complete Windows/macOS combinations across
`windows_windows`, `windows_macos` and `macos_macos`. Each combination retains
six independent format rows and twelve hard gates; score averaging is
structurally forbidden. No combination is selected. Bundled FFmpeg decodes all
smoke formats but lacks end-to-end range, 30-sample/two-hour and production
release proof. The native combination retains exact Windows Ogg/Opus failure
`0xC00D36C4` and native macOS full-source preparation. Pure Go retains absent
acceptable AAC, full-scan MP3 seek and missing Ogg random seek. Deterministic
range/cache faults are labeled substrate-only rather than candidate evidence.
Physical pairing/timing/RSS rows remain explicit `not-run` gates in manual epic
`EPIC-260714-th54l3`. Contract tests passed 16/16; hosted CI run `29451972760`
passed all four jobs. Local full acceptance is not mislabeled green because the
installed FFmpeg lacked the `libvorbis` encoder required by two pre-existing
media tests; the hosted coordinator installed the required package and passed.
Progress is 88/205 overall and 88/186 engineering; strict execution advances
to `TASK-260712-2eympi`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-2eympi` publishes the required
production no-go ADR on engineering head `3e40b14`, merged by PR #118 as
`b253b39`. The accepted comparative matrix permits no selection, so the ADR
names no codec, container or complete combination and keeps the production
decoder registry empty. A machine-readable handoff freezes candidate-neutral
`stream_variants`, authenticated single-range transport, SHA-256 integrity,
512/64/128 MiB cache ceilings, 1 MiB chunks and PCM ring, generation-safe seek,
48 kHz stereo float PCM, coordinator timing gates, exact fixture hashes, range
profiles, three pairings, sample counts and release obligations. Downstream
engineering may implement schema, state machines, collectors and deterministic
test doubles; production variant generation, playback, Store submission,
fallback download and sandbox weakening remain fail-closed. Bundled, native and
pure-Go rejection reasons are retained, and a replacement ADR requires one
exact combination to pass every format, hard gate, pairing and release gate.
Validator and negative tests passed 19/19; hosted CI run `29452694269` passed
all four jobs. Physical evidence remains in manual epic
`EPIC-260714-th54l3`. The codec/player spike story is closed as an explicit
no-go, not a production approval. Progress is 89/205 overall and 89/186
engineering; strict execution advances to `TASK-260712-2rlkp7`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-2rlkp7` freezes
`p2-targets-inbox-parity.v1` on engineering head `22e2aa4`, merged by PR #120
as `100678f`. The contract extends Phase 1 immutable target snapshots, history
cursors, media ACL, secure callbacks and moderation authority; parallel ACL,
history, moderation and Telegram-owned queues are forbidden. It defines atomic
N-recipient create, targeted `audio_track` queue/replace without broadcast
fallback, whole-request mixed-version `422 unsupported_targets`, nine exact
inbox-eligible missed reasons, a bounded 30-day TTL, stable entry and cursor
fields, manual replay as a new transmission with depth-eight lineage, local
dismiss versus sender delete, content-policy reauthorization and zero late
autoplay. Non-targets in the same Air get the 404 nonexistence surface and no
main-program state change; new or replacement members never inherit old media.
Reports have reporter-local effects only. Global quarantine is a distinct
reversible audited operator decision, while reviewed delete/disable remain the
existing terminal enforcement paths; report counts cannot cause global denial
of service. HTTP, Windows, macOS and Telegram share one service contract.
Validator and negative tests passed in the 31/31 contract suite; hosted CI run
`29453630078` passed all four jobs. Progress is 90/205 overall and 90/186
engineering; strict execution advances to `TASK-260712-1c34fe`.

Checkpoint 2026-07-15 (accepted): `TASK-260712-1c34fe` extends the existing
Phase 1 transmission transaction into the common Phase 2 explicit-target
service on engineering head `5b69232`, merged by PR #122 as `1cc0759`.
Application requests now carry only random `trf_` capabilities; SQLite stores
only their digests plus ActorContext credential scope and server-side target
binding. Resolution rechecks the current own/Air domain and Pulsar binding
generation, then sorts opaque selectors, expands them to exact installations,
deduplicates and applies include-origin before sealing the immutable snapshot.
Forged, copied, expired, outside-domain and stale references share the 404
nonexistence surface. Any mandatory online explicit target missing the one
clip/future-track capability policy aborts the whole create with
`422 unsupported_targets`; details expose only opaque references and sorted
capability names, and no partial transmission/request row is committed.
Production Telegram supplies verified identity only: the common service owns
the current-domain-minus-origin personal rule and mints the same references.
Legacy unbound rows refuse an unrepresentable N-recipient personal action
instead of rewriting it to broadcast. Existing replay creates a new
transmission and re-resolves current authority; adapters contain no target
business logic. Local pinned coordinator, Windows and Swift suites passed;
hosted CI run `29455392790` passed all four jobs. Physical hardware evidence is
not claimed. Progress is 91/205 overall and 91/186 engineering; strict
execution advances to `TASK-260712-2bk0vy`.

Checkpoint 2026-07-16: `TASK-260712-2bk0vy` extends the immutable Phase 1
target tuple with canonical capability digest and resolution time, then creates
exactly one inbox projection in the same transaction as each of the contract's
nine eligible terminal receipts. Keyset reads revalidate the exact current
installation generation without joining membership or Air state; replacement
bindings and later members inherit neither old inbox rows nor media bytes.
Expiry, dismissal/consumption fields, replay root/depth and canonical
media/moderation revocation are additive, while rollback-safe defaults and
startup backfill reconcile rows written by the previous coordinator. Local
store/HTTP, targeted race, contract/acceptance, vet and exact previous-head
rollback suites passed. PR #124 landed engineering commit `a2814c8` at merge
`80e892b91afdfa8203eab7c19b14b294a3b7db2d`; hosted run `29456807669`
passed coordinator, Swift, Windows and packaged-probe jobs. No real-hardware
claim is made. Progress is 92/205 overall and 92/186 engineering; strict
execution advances to `TASK-260712-2ctf3x`.

Checkpoint 2026-07-16 (accepted): `TASK-260712-2zoy4u` keeps report, block,
delete and disable enforcement on the canonical Phase 1 stores and services.
Creating or reusing a report atomically makes the media unavailable only to
the reporting actor across inbox, replay, direct descriptor/range access and
future target resolution; late receipt/backfill follows the same terminal
state. Scheduler cancellation is scoped to that media and reporter, so shared
Telegram evidence targets, the owner and unrelated recipients remain
unaffected, while sender-facing receipts redact the internal report reason.
Canonical delete and disable retain their existing global terminal behavior,
and current content-policy consent remains the upload/send gate. Local
contract, acceptance, affected Go, vet, targeted race, previous-head, Swift
and Windows suites passed. Hosted run `29462753677` passed all four jobs on
exact engineering head `36f51e0`; PR #130 landed at merge `fd6a5df`. The two
local OGG/Vorbis fixture cases remain a host-ffmpeg limitation and passed in
the authoritative hosted coordinator job. No real-app or hardware result is
claimed. Tracking head `018bcd7` passed all four hosted jobs in run
`29462981598`; PR #131 landed at merge `8a2defa`. Progress is 95/205 overall
and 95/186 engineering; strict execution advances to `TASK-260712-2vipy3`.

Checkpoint 2026-07-16 (accepted): `TASK-260712-2vipy3` adds one additive,
localized presentation projection for opaque explicit targets, inbox, history
and paginated receipts plus equivalent fail-closed Swift and Windows models.
The shared command boundary permits only current server-derived own, current
Air and explicit audiences; prunes expired targets and inbox authority; keeps
requested/effective delivery and partial receipts distinct; and exposes
replay, dismiss, delete, report and mute only from the latest returned action
capabilities. Inbox moderation resolves through the row's server-returned
`history_item_id`; stale/offline/error snapshots retain readable rows but no
mutation authority, and there is no late-autoplay command. Local affected Go,
vet/race, Windows full/race, Swift 226/226, contract and acceptance validators
passed. Exact engineering head `5968046` passed all four hosted jobs in run
`29464453352`; PR #132 landed at merge `45f27ac`. No native final-view or
real-app/hardware result is claimed. Tracking head `6400b22` passed all four
hosted jobs in run `29464705100` after a failed-job rerun confirmed two old
Windows async timing flakes; PR #133 landed at merge `aa04a01`. Progress is
96/205 overall and 96/186 engineering; strict execution advances to
`TASK-260712-2nto40`.

Checkpoint 2026-07-16: `TASK-260712-2nto40` is accepted on exact engineering
head `382e055`. Native macOS SwiftUI now exposes authenticated current-Air and
opaque explicit targeting, versioned consent, paginated inbox/history and
receipts, explicit replay/dismiss/delete/report/mute actions, durable exact
targeted-send retry, fail-closed reconnect semantics, EN/RU copy and an
explicit no-autoplay boundary. Queue/replace remains visibly unavailable until
the later streamed-track contract exists rather than inventing a fallback.
Local Xcode tests passed 231 tests in 38 suites; the presentation, coordinator
contract and acceptance validators passed. Hosted run `29466777419` passed all
four jobs; PR #134 landed at merge `22f7175`. No hands-on VoiceOver, packaged
app, audible playback or physical-hardware result is claimed; those remain in
`EPIC-260714-th54l3`. Progress is 97/205 overall and 97/186 engineering; strict
execution advances to `TASK-260712-cuplon`.
Tracking head `171e60e` then passed all four hosted jobs in run `29467019035`;
PR #135 landed at merge `986cf0d`.

Checkpoint 2026-07-16: `TASK-260712-cuplon` is accepted on exact engineering
head `0b4cd04`. The packaged Windows shell now exposes authenticated current-Air
and opaque explicit targets, include-origin, delivery choice, paginated
inbox/history/receipts and explicit replay, dismiss, delete, report and mute
commands through the shared fail-closed model. Durable retries freeze the exact
sorted target set and idempotency keys without displaying or logging opaque
references. Reconnect retains readable rows without stale action authority;
automatic refresh preserves an active selection only until its capability
expires, and no inbox read can autoplay. EN/RU keyboard, Win32 accessibility
source seams and 96/120/144/192-DPI layout are deterministic tests; hands-on
Narrator, packaged-app, audible and real-hardware evidence remains exclusively
in `EPIC-260714-th54l3`. Local full/race/vet, amd64/arm64 cross-build, contract
validators and pinned Windows acceptance passed. Hosted run `29468731725`
passed all four jobs; PR #136 landed at merge `15f675e`. Progress is 98/205
overall and 98/186 engineering; strict execution advances to
`TASK-260712-1vklop`.

Checkpoint 2026-07-16: `TASK-260712-1vklop` is accepted on exact engineering
head `1b15cafbabd7543e5a7ee4d96af977d4abb1b994`. A fail-closed manifest maps 19
B5-B7 repository invariants to executable coordinator, Windows, macOS and
Telegram anchors. Adversarial coverage proves non-target nonexistence even
with known IDs, immutable deduplicated N-recipient snapshots without origin or
broadcast fallback, new-member isolation, terminal TTL without replay,
cursor/binding isolation, atomic mixed-version rejection, current consent,
reporter-local protection, authority-driven revocation and additive
migration/previous-head rollback. All three presentation surfaces consume one
fixture and explicitly keep targeted tracks unsupported until the downstream
streamed-track story. Local all-suite acceptance passed all 12 commands,
including full/race/vet, Windows amd64/arm64 builds and 232 Swift tests. Hosted
run `29470131117` passed all four jobs; PR #138 landed at merge `029346c`.
Real packaged apps, physical hardware, Narrator/VoiceOver, audible behavior,
real-network denial and mixed fleet remain manual-required in
`TASK-260712-3u5cdn`. Progress is 99/205 overall and 99/186 engineering;
strict execution advances to `TASK-260712-20cuna`.

Checkpoint 2026-07-16: `TASK-260712-20cuna` is accepted on exact engineering
head `43534c88540644f6b1477fab8c8cb1e0b3ad96f3`. The final versioned handoff
freezes eight API/rights surfaces, seven coordinator-first rollout stages, the
atomic mixed-version policy, additive schema reconciliation, single-writer
drain/rollback without destructive down-migration, preserved target/inbox/
policy/moderation tables and eleven streamed-track, acceptance and future E2EE
consumers. The validator rejects early targeted-track enablement, broadcast or
plaintext fallback, missing downstream tasks and promotion of either manual
gate. Local all-suite acceptance passed all 12 commands and 41 contract/unit
tests; hosted run `29470807661` passed all four jobs. PR #140 landed at merge
`e51c937`. No production rollout was executed: `TASK-260712-3u5cdn` and
`TASK-260712-3qybi2` remain manual-required. Progress is 100/205 overall and
100/186 engineering; `STORY-260712-ob1tx2` is complete and strict execution
advances to `TASK-260712-1n5fks`.

Checkpoint 2026-07-16: `TASK-260712-1n5fks` is accepted on exact engineering
head `b64a671d2356dad7d021905a61f498e9cd94ac18`. The additive model persists
original audio-track metadata, immutable candidate variants, strong whole and
chunk integrity, pinned canonical profiles, chunk-aligned seek maps,
main-program resume pointers, queue state, audible progress and playback/seek
generations without changing Phase 1 media, transmission, inbox, history,
Spotify or legacy session authority. Worker publication/revocation and
playback updates are revision-conditional; production codec selection remains
fail-closed under `p2-codec-player-adr-handoff.v1`. Store vet/full tests, 41
contract tests and exact previous-binary rollback passed locally; hosted run
`29471845396` passed all four jobs. PR #142 landed at merge `54780069`. No
production decoder, real-app playback or hardware result is claimed. Progress
is 101/205 overall and 101/186 engineering; strict execution advances to
`TASK-260712-31rkpe`.

Checkpoint 2026-07-16: `TASK-260712-31rkpe` is accepted on exact engineering
head `ea2d6d42eae0999ca8f311ffca8a440db78db562`. The shared contract adds
generation-safe load, resume, seek, pause and cancel commands plus ready,
started, progress, rebuffer, failure, ended and cancelled events across Go,
Swift and the Windows mirror. Fifty-one shared goldens pin opaque
credential-free manifests, integrity fields, coordinator-clock readiness and
start barriers, stale/duplicate/reordered rejection and explicit
sender-selected mixed-version handling. Production runtimes still do not
advertise `stream_track_v1`, so no decoder/player or real-app result is
claimed. Hosted run `29473326227` passed all four jobs; PR #144 landed at
merge `0b9fc7d6`. Progress is 102/205 overall and 102/186 engineering; strict
execution advances to `TASK-260712-2ogntd`.

Checkpoint 2026-07-16: `TASK-260712-2ogntd` is accepted on exact engineering
head `00a269747fcb17133304e57ba0f76976a20f1daf`. Authoritative projections
cover per-actor/per-orbit upload starts/input, original and canonical retained
bytes, processing temp/concurrency, range requests, actual egress and active
reservations. Default and revisioned override quotas are deterministic; an
admitted playback owns a bounded 2x reservation and survives later quota
reduction, while amplification fails explicitly. Startup/five-minute
reconciliation releases stale leases, and exact metrics/adjustments require a
still-live operator capability inside the same SQLite transaction. Local full
store/command, focused race, fault-injection and exact-predecessor rollback
tests passed; hosted run `29475162175` passed all four jobs. PR #146 landed at
merge `15ebd3d5`. Production stream capability remains disabled and no real
traffic is claimed. Progress is 103/205 overall and 103/186 engineering;
strict execution advances to `TASK-260712-285pag`.

Checkpoint 2026-07-16: `TASK-260712-285pag` is accepted on exact engineering
head `7b307559b2b455f89b8ab206bc96802d79cf92b2`. App uploads now admit
content-policy-consented `audio_track` sources up to 500 MiB and two hours,
then perform fixed-prefix signature routing and one file-only, resource-capped
probe without entering the Phase 1 speech high-pass/compressor/loudness or WAV
transcode path. Immutable original duration/size/hash/format metadata and
processing temp/high-water accounting are persisted. Because the accepted
codec ADR selected no production profile, a valid track deterministically
finishes `codec_profile_unavailable`; no candidate is invented and no staged
or ready variant, generated WAV or production capability appears. Retry,
worker crash, oversized input, current consent, delete race, zero-artifact
cleanup and exact-predecessor rollback passed locally, including focused race;
the complete hosted run `29476335634` passed all four jobs. PR #148 landed at
merge `4749a76ae0576cab628171528a01912ba025e0ea`. No real-app playback,
production codec/player or hardware result is claimed. Progress is 104/205
overall and 104/186 engineering; strict execution advances to
`TASK-260712-3lf8r0`.

Checkpoint 2026-07-14 (in progress): `TASK-260712-16zfvu` now has a strict
machine-readable legal/operations approval contract and a seven-group human
checklist. Repository and live-site audit found usable candidates for the
Relux Works legal identity, general privacy/legal contacts, Armenian law and
Partner Center product identity, but initially had no explicit Pulsar approval,
moderation roster, support/moderation ownership, hosting/data locations,
markets, final submit authority or policy reviewer. More critically,
`barycenter.live` routes
`/privacy`, `/terms` and `/support` return the homepage bytes rather than real
documents, while the general Relux policy does not disclose Pulsar media,
Telegram/Spotify, retention or moderation. The new validator rejects unknown
fields, unowned approvals and placeholder values. Manual Store submission now
runs `--require-approved` before installing `msstore` or downloading an MSIX;
ordinary engineering remains unblocked. Local coordinator vet/full tests,
focused race, Windows vet/tests, Swift release build, board validation and diff
checks passed; hosted run `29335621951` passed all four jobs on code head
`18eae3f`, and tracking head `e9542b7` passed all four jobs in run
`29335884943`, and doc-only head `174a236` passed all four jobs in run
`29336133129`. Ivan Oparin then approved the observed Relux Works candidates,
named himself common owner, and supplied product mailboxes, canonical future
URLs, United States data regions, age 13, Armenian law/courts, English control,
GMT+4, reviewers and Store authorities. Three groups are now approved: legal
identity/controller, contacts/public URLs and Partner Center/submission. The
partial-approval head `86c7c4a` passed coordinator, node-core, pulsar-win and
the signed packaged probe in hosted run `29337160625`. Ivan Oparin subsequently
approved the proposed best-effort defaults for the remaining four groups:
Relux-operated United States hosting and backup with no subprocessors; all
lawful Microsoft Store markets except sanctioned, embargoed or prohibited
jurisdictions; Monday-Friday 10:00-19:00 GMT+4 moderation with two-business-day
normal and 24-hour urgent targets; and no separate counsel requirement with
Ivan Oparin as EN/RU reviewer. All seven groups and `--require-approved` now
pass locally. Exact head `3b12371` passed all four hosted jobs in run
`29338589269`; the task is accepted. Progress is
30/205. Tracking head `5af1b56` passed all four hosted jobs in run
`29339017452`; PR #29 landed at merge
`e588fc9b727d6264c289f69cc97ea77e4987f939`, and strict execution advanced to
`TASK-260712-2kec2s` from synchronized `main`.

Checkpoint 2026-07-14: `TASK-260712-2cdjq8` closes the P1 transmission story
with one stable rollout/handoff entry point. It records the frozen strict HTTP
request, immutable target ACL, closed receipt/status vocabulary, privacy-
bounded presence, DND/block precedence and whole-transmission legacy downgrade
without creating a second normative contract. The rollout is honestly
coordinator-first and caller-surface-last because no transmission-wide runtime
feature flag exists; current macOS and Windows clients advertise
`media_clip_v1` but withhold overlay/interrupt capabilities until their mixer
tasks land. Rollback requires withdrawal of every creator and a zero count for
`transmissions.completed_at = 0`, forbids manual scheduler/receipt mutation and
preserves every additive table for roll-forward. A document guard verifies the
contract, diagrams, evidence links and runbook/protocol entry points. Local
coordinator vet/unit/race, Windows vet/unit/race and amd64/arm64 builds, Swift
release build, board validation and diff checks passed; hosted run
`29334550550` passed all four jobs on exact head `cd234c9`. Physical playback,
audibility, packaging and hardware timing remain unpassed in
`EPIC-260714-th54l3`.

Checkpoint 2026-07-14: `TASK-260712-2qc27p` closes the deterministic
transmission regression matrix. Strict HTTP tests reject caller-controlled
acceptance/ID fields and cover origin defaults plus the exact 60,000/60,001 ms
overlay boundary. A persisted table covers all 35 valid terminal
status/reason pairs. Multi-target integration proves one T derived from fresh
maximum RTT; mixed fleets use one visible legacy delivery for every target;
terminal convergence clears the scheduler timer and makes stale wakes inert.
The exact rollback gate now runs both the pre-scheduler `0c1e194` coordinator
and pre-transmission-schema `2aa97c2` coordinator while preserving
transmissions, target ACL, legacy state and scheduler state. Coordinator
vet/full/race, 20x shuffled regressions, Windows vet/unit/race and amd64/arm64
cross-build, Swift release build and both pinned rollback cases passed locally;
hosted run `29333494719` passed coordinator, node-core, pulsar-win and signed
packaged-probe on exact head `c60bd99`. The criterion map explicitly leaves
real-app playback, audible non-overlap, physical p95 skew and late-work
inaudibility unpassed in `EPIC-260714-th54l3` / `TASK-260712-2hodti`.

Checkpoint 2026-07-14: `TASK-260712-31vvjt` now has one durable scheduler per
persisted orbit or approach playback domain. Overlay and interrupt share an
`accepted_at` plus ULID FIFO, including equal-time ties and opposite approach
origins. The additive scheduler state persists the exact three-second prepare
barrier, RTT-derived coordinator T and 100 ms late window; monotonic timer
handling prevents wall-clock rollback from extending a deadline. Runtime
rechecks exact binding, block, DND, liveness, immutable/current capability and
fresh per-socket RTT evidence. Generation-bound receipts, bounded start/end/
cancel watchdogs, strict `T < expires_at`, safety stop, restart reconciliation,
media lifecycle, target revoke, leave and approach split flows converge without
inventing interrupt fallback. Legacy `after_current` stays on the Session FSM
with exact targets, idempotent enqueue/cancel and generic media ACL URLs. Local
coordinator vet/full test/focused race/20x shuffled stress/build and exact
previous-HEAD rollback passed; Windows vet/test/race/cross-build and macOS
release build passed. Hosted run `29331940948` passed coordinator, node-core,
pulsar-win and signed packaged-probe on exact code head `d0e1b92`. This is
deterministic engineering evidence only: no real-app playback, audible mixing,
measured multi-node p95 skew, packaged installation or physical-hardware result
is claimed; those checks remain in `EPIC-260714-th54l3`.

Checkpoint 2026-07-14: `TASK-260712-2bbz13` now has a generation-safe Windows
`media_clip_v1` client behind a delivery-capability-gated mixer seam. It
performs authenticated same-origin fetch with redirect refusal, bounded exact
size and SHA-256 verification, decoder-ready WAV validation, coordinator-clock
scheduling with the exact 100 ms late window, idempotent prepare/play/cancel,
exactly-once terminal receipts and cache cleanup. Durable local DND and a
privacy-bounded presence projection use atomic Windows persistence; legacy
`play_voice` and `solo_voice` remain supported with sanitized failures. Network
I/O, decode, persistence and scheduling stay off the WebSocket read and render
callback paths. Deterministic lifecycle, ordering, cancellation, deadline,
redaction, persistence, fetch-policy and race regressions are green alongside
Windows amd64/arm64 cross-builds, coordinator compatibility and Swift build.
The first hosted attempt found a test-only POSIX permission assertion on
Windows; `219306c` made that assertion platform-correct without weakening the
runtime. Exact-code run `29326895259` then passed Windows unit/cross-build,
native helper and signed-MSIX construction, coordinator and node-core. This
does not advertise `overlay_mix_v1` or `interrupt_resume_v1` and claims no
installation, audible playback, Windows 10/11 or physical-hardware evidence;
those remain in their later engineering tasks and manual epic
`EPIC-260714-th54l3`.

Checkpoint 2026-07-14: `TASK-260712-26ip33` now has a generation-safe macOS
`media_clip_v1` client runtime behind a delivery-capability-gated mixer seam.
It performs same-origin authenticated fetch with redirect refusal, bounded
exact-size and SHA-256 verification, decoder-open readiness and exact duration,
coordinator-clock scheduling with the frozen 100 ms late window, idempotent
prepare/play/cancel handling, exactly-once terminal receipts and cache cleanup.
Coordinator sends are serialized off caller queues; node-local DND and the
privacy-bounded presence projection persist across restart. This build does
not advertise `overlay_mix_v1` or `interrupt_resume_v1`: those stay gated until
their dedicated mixer tasks implement exact behavior. Deterministic Swift
regressions cover lifecycle, stale/cancel races, capability gating, DND,
presence and production downloader policy. Swift build/parser, coordinator
vet/tests and Windows vet/tests/cross-build are green. Local `swift test`
cannot enter the suite because this host toolchain lacks the existing
`Testing` module. Hosted run `29324579129` passed all four jobs on exact code
head `9622e00914195a5a17e4420cc1de5d8ce7a16921`; node-core reported 145 tests
passed. Root hardening added an absolute streamed-body cutoff and async-safe
test locking. No manual audio, packaged-app or hardware evidence is claimed.

Checkpoint 2026-07-14: the `TASK-260712-2qpp6w` implementation candidate now
exposes strict control-authenticated create/cancel and actor-authenticated
status endpoints. One immediate SQLite transaction reauthenticates the bearer,
resolves live media and audience bindings, applies block/DND/presence and
capability policy, consumes hashed fallback proofs and seals idempotency plus
immutable target rows. Whole-delivery overlay downgrade, explicit single-use
interrupt fallback, non-disclosing visibility and generation-bound sender
cancel handoff are covered. Root review added exact omitted-versus-empty slot
handling, empty-selector and corrupt-link fail-closed behavior, actual-socket
presence, an exact current-credential witness and credential-aware cancellation
output. Coordinator vet/full test/race, 20x idempotency and confirmation stress,
Windows vet/test/race/cross-build, Swift build, diff/resource comparison and
task-board validation are green. The attached outcome explicitly leaves FIFO/
barrier/disarm delivery to `TASK-260712-31vvjt` and claims no real-app or
physical-hardware result. Exact remediated code head `4d737bc` passed all four
hosted jobs in run `29322386396`; final tracking head `e0900ed` passed run
`29322606238`, and PR #23 landed at `30f1c55`.

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

Checkpoint 2026-07-14: `TASK-260712-1sae4q` is accepted on branch
`task/task-260712-1sae4q-media-delete-retention-cleanup`. It implements atomic
owner-orbit control deletion, immediate read revocation, the frozen
`media_lifecycle_v1` cancellation outbox, seven-day clip expiry, crash-safe
canonical and temporary cleanup, 90-day content-free audit pruning, health
metrics and the backup/privacy handoff. Coordinator vet, full tests, full race,
the exact `451e50b` predecessor round-trip, and portable Windows vet/test/build
are green. Hosted CI run `29304495443` passed coordinator, macOS Swift,
portable Windows and the signed-MSIX probe on code commit `a627593`. Root delta
review closed the unlink-before-directory-fsync crash window. The production
cancellation sink remains deliberately pending for the later transmission
tasks. The final tracking commit passed all four jobs again in CI run
`29304654747`; PR #14 landed at
`fe8e73c6c8d7dd2f05a3ff0acc4926ef30afa169`, and strict execution advanced to
`TASK-260712-3mcof4`. No manual real-app or hardware result is claimed.

Checkpoint 2026-07-14: `TASK-260712-3mcof4` is accepted on code commit
`49d21cdd92600364a48c155f88f04045d859374e`. It exposes
control-owner `GET /v1/media/{id}` and a fail-closed immutable
`(media_id, orbit_id, actor_id, slot)` target-snapshot reader for node access.
It re-resolves live credentials and media state, holds the second immediate
SQLite authorization through canonical descriptor acquisition, returns uniform
not-found responses for unknown/foreign/deleted/expired reads, refuses symlink
storage, keeps credentials and request identities out of logs, and isolates the
legacy node-only current-approach bridge from the generic ACL. Production node
reads remain deliberately closed until `TASK-260712-1aprcb` supplies persisted
transmission targets; `TASK-260712-gj0cko` owns the later integrated lifecycle
path. Focused race tests, full coordinator vet/test/race, the exact predecessor
rollback suite, pulsar-win vet/test/Windows cross-build and task-board
validation are green. Local `node-app` Swift tests still stop at the known
workstation `no such module 'Testing'` toolchain gap; hosted macOS CI remains
the authoritative gate. Hosted CI run `29305916096` passed coordinator,
macOS Swift, portable Windows and signed-MSIX jobs. Inline root delta-review
closed the authorization-to-open TOCTOU, and 20x race/HTTP stress passed on the
reviewed bytes. The final tracking commit passed all four jobs again in CI run
`29306064452`; PR #15 landed at
`0f3148a379258b9af1934d3d6e582e7998c40f59`, and strict execution advanced to
`TASK-260712-12ojcb`. No manual real-app or hardware result is claimed.

Checkpoint 2026-07-14: implementation of `TASK-260712-12ojcb` is under hosted
review on code commit `908f89a2ebe96e5ac5e32c2979e743bb167a8b9e` from branch
`task/task-260712-12ojcb-telegram-submitmedia-compat`.
Telegram acceptance now atomically creates the transport-neutral
`media_items(source=telegram)` row and the legacy `media` row with one media
ID, including feature-off projection of the authoritative legacy member. Raw
bot download is physically capped at 20 MiB plus one detection byte and enters
the same singleton `SubmitMedia` service as app uploads; the
ready canonical WAV is mapped back to acceptance-ordered legacy `KindVoice`,
`play_voice` and authenticated `/media/{id}.wav`. Common ingest failure codes,
personal/broadcast defaults and the exact accepted/ready/failure bot replies
remain covered. Failed private Telegram sources stay available for the legacy
operator-debug contract and are removed by a retryable retention sweep. Local
full `go test`, `go vet`, full race, focused race, 20x focused stress, exact
previous-head media-processing rollback and Linux amd64 CGO-free build are
green. Hosted CI run `29307473249` passed coordinator, macOS Swift, portable
Windows and packaged-MSIX jobs on PR #16. Root delta-review found no contract,
privacy, retention or compatibility regression. Final tracking CI run
`29307610519` passed the same four jobs; PR #16 landed at
`0d6863c462111da6ed27f851a636e40d95100d73`, and strict execution advanced to
`TASK-260712-gj0cko`. No manual real-app or hardware result is claimed.

Checkpoint 2026-07-14 for `TASK-260712-gj0cko`: generic media is now the
logical authority for linked Telegram compatibility rows; terminal state,
safe canonical/private-source cleanup and durable cancellation receipts remain
consistent through exact rollback and roll-forward. The current serial session
runtime disarms queued copies, stops an active voice and durably advances once;
macOS and Windows generation-gate in-flight voice work and order insertion
pause before the following load. Local coordinator vet/test/race, the complete
pinned predecessor suite, portable Windows vet/test/race and amd64 cross-build,
plus local macOS compilation are green. Hosted CI run `29309915183` passed
coordinator, authoritative macOS Swift tests, portable Windows and signed-MSIX
jobs on PR #17. Root delta-review also excluded linked rows from the unsafe
legacy sweeper and pinned cleanup to the canonical/Telegram roots. Final
tracking CI run `29310143986` passed the same four jobs; PR #17 landed at
`9f2aea8e5b9200d1e4077a5576dde18f8051bba5`, and strict execution advanced to
`TASK-260712-3huupe`. No manual real-app or hardware result is claimed.

Checkpoint 2026-07-14 for `TASK-260712-3huupe`: the automated acceptance
delta now exercises every accepted phase-one codec family through the common
service, joins resumable HTTP upload to ACL/delete/cleanup, expands sanitized
adversarial failure coverage, and restarts SQLite plus the lifecycle service
after an unlink-before-receipt crash. The story acceptance map is recorded in
`docs/analysis/p1-media-ingest-acceptance-evidence.md` and mirrored to the task
board. Local coordinator vet/test/race, focused race stress x20 and the exact
pinned predecessor suite are green. Hosted CI run `29311147090` passed the
real-ffmpeg eight-variant codec matrix, compact 181-second AAC rejection,
macOS Swift tests, portable Windows and the signed-MSIX job on PR #18. Inline
root delta-review found no production-code regression or uncovered
deterministic story criterion. Final tracking CI run `29311329355` passed the
same four jobs; PR #18 landed at
`cfe12ed211e9f763d683a3fa3ace9cf8f4f1efc3`, and strict execution advanced to
`TASK-260712-jolzhh`. No manual real-app or hardware result is claimed.

Checkpoint 2026-07-14 for `TASK-260712-jolzhh`: the implementation draft now
provides one authoritative upload retry/state/retention/compatibility handoff,
explicit cross-story ownership, and truthful rollout, readiness, rollback and
roll-forward instructions. The audit found that generic app clips used the
approved seven-day retention while the Telegram config default and deployment
templates still used 30 days; those defaults now converge on seven days while
an explicit compatibility override remains honored and tested. Local focused
tests, full coordinator vet/test/race, the exact pinned predecessor suite,
portable Windows vet/test/cross-build, Swift build, YAML parsing, documentation
link checks and task-board validation are green. Local Swift tests still stop
at the known workstation `no such module 'Testing'` toolchain gap. Hosted CI
run `29312221521` passed coordinator with live ffmpeg and pinned rollback,
authoritative macOS Swift tests, portable Windows and the signed-MSIX probe on
code commit `fc99fac`. Inline root delta review found no unresolved contract,
security, migration or handoff issue. Final tracking CI run `29312378238`
passed the same four jobs; PR #19 landed at
`c4cb324bb4e783e97bb1fbf1bb61efef9dfbf10f`, completed the P1 media ingest
story, and advanced strict execution to `TASK-260712-51y5k9`. No manual
real-app or hardware result is claimed.

Checkpoint 2026-07-14 for `TASK-260712-51y5k9`: the normative
`p1-transmission-v1` note now fixes strict create/status/cancel HTTP shapes,
immutable audience and visibility rules, coordinator-owned acceptance order,
origin defaults, target and aggregate states, whole-transmission overlay
downgrade, explicit interrupt fallback confirmation, five-minute speak-now
expiry, the exact three-second prepare barrier and RTT formula, a 100 ms stale
start window, generation-safe WebSocket payloads, DND/block ownership and
click-free active delete. A Go contract guard parses all 23 JSON examples and
pins the critical decisions. Local full coordinator test/vet, focused race,
portable Windows tests, diff and board checks are green. Hosted CI run
`29314060965` passed coordinator with pinned previous-head compatibility,
authoritative macOS NodeCore, portable Windows and the signed packaged probe
on implementation commit `605859b`. Inline review closed aggregate reason,
cancel/start race, capability refresh and DND acknowledgement gaps. The task
is accepted. Final tracking CI run `29314299856` passed the same four jobs;
PR #20 landed at `2aa97c2d08cb93b110200ae159fd43265410ff5a`, and execution
started `TASK-260712-1aprcb` from that clean main. No manual real-app or
hardware result is claimed.

Checkpoint 2026-07-14 for `TASK-260712-1aprcb`: the coordinator now persists
immutable accepted transmission and exact target snapshots, trusted FIFO and
expiry fields, requested plus effective delivery, generation-safe receipt
transitions, deterministic aggregate lifecycle, actor/orbit blocks and layered
DND state. Generic media downloads are authorized from the stored target
identity rather than current membership, and production descriptor opening
rechecks both the exact target and active block within the immediate database
transaction. The additive schema deliberately avoids legacy identity foreign
keys so transmission history survives source-orbit dissolution and remains
readable by the exact previous binary. Fresh/migrated schema tests, concurrent
CAS and block-versus-open races, production HTTP ACL coverage, full Go
test/vet/race, deterministic integration coverage and the pinned previous-head
rollback suite are green. Inline review closed the descriptor-open race and
strengthened rollback coverage. Hosted CI runs `29315987760` and `29316416647`
both passed coordinator, authoritative macOS NodeCore, Windows unit/cross-build
and signed packaged-probe jobs on implementation commits `ab9b9b7` and
`a4610b4`. Final tracking commit `8a925f0` passed all four jobs again in run
`29316678680`; PR #21 landed at
`35d9974e6a2212b6757e6d053d8b896a652ec4f7`, and strict execution advanced to
`TASK-260712-1g70av`. No manual real-app or physical-hardware result is claimed;
those checks remain in `EPIC-260714-th54l3`.

Checkpoint 2026-07-14 for `TASK-260712-1g70av`: the canonical Go codec and its
byte-pinned Windows mirror now carry `prepare_media`, `play_media_at`,
`cancel_media`, `presence_update`, `media_ready`, `media_started`,
`media_ended`, `media_failed`, `media_cancelled` and `set_dnd`; the Swift
`Message` mirror carries the same cases. Thirty-nine shared golden envelopes
pin all message shapes, delivery-conditional fields and DND/presence additions,
while explicit compatibility tests keep `play_voice` and `solo_voice` intact.
Registration now validates unique ASCII-sorted capability names, retains
unknown additive values and replaces the authenticated connection snapshot on
reconnect; the serialized loop keeps the exact target capability set. Current
clients deliberately advertise no clip flag until their implementation tasks.
Inline review corrected malformed legacy golden `msg_`/`el_` identifiers and
added recursive Crockford-ULID guards, then aligned `invalid_dnd_revision` with
the frozen DND vocabulary. Full coordinator and Windows vet/test/race, Windows
cross-build, Swift build, JSON, mirror and board gates are green. Hosted CI runs
`29318171135` and `29318440712` both passed coordinator compatibility,
authoritative macOS Swift tests, Windows unit/cross-build and the signed
packaged probe on commits `8fb9465` and `36c15fa`. PR #22 is ready and
mergeable. No manual real-app or physical-hardware result is claimed; those
checks remain in `EPIC-260714-th54l3`.

Checkpoint 2026-07-16 (accepted): `TASK-260712-3aj8w2` adds a candidate-neutral
macOS streamed-track range/cache/player seam without weakening the accepted
codec no-go. Exact bearer-authenticated single ranges are capped at 1 MiB;
verified chunks use installation-secret HMAC names, atomic fsync/rename,
duration-independent 512 MiB global, 64 MiB variant and 128 MiB pinned limits,
LRU refill and durable delete/disable tombstones. An injected decoder writes
48 kHz stereo float PCM through backpressure into a fixed 1 MiB SPSC ring.
Exact generations fence ready, scheduled start, pause, seek, resume, progress,
rebuffer, cancel and drain-before-ended; only the render consumer applies ring
cuts, and source checks keep allocation, locks, queues, disk, network and decode
off render. Spotify, clip, overlay, interrupt, Airfoil and output composition
remain untouched, while production decoder registration and
`stream_track_v1` advertisement remain disabled. Focused tests passed 6/6;
the full macOS suite passed 248 tests in 40 suites, release build and frozen
handoff validators passed. Exact engineering head
`e6f0685f29e6be0dec95b0ea89b7c5463ee1206b` passed all four hosted jobs in run
`29487762262`; PR #158 landed at merge
`606994898a6e6873c3cc8ed330c82a236bbd3f01`. Real p95 timing, process RSS,
two-hour/audible/packaged/hardware evidence remains unclaimed in manual task
`TASK-260712-1fpb9q`. Progress is 109/205 overall and 109/186 engineering;
strict execution advances to `TASK-260712-wt2n7m` from synchronized `main`.

Implementation checkpoint 2026-07-16 (active): `TASK-260712-wt2n7m` now routes
Telegram explicit Barycenter/Pulsar selections through the common opaque target
reference and transmission services, binds include-origin in an additive
rollback-safe callback companion, and preserves current-Air and immediate
legacy voice defaults through the existing shared contract. Audio/document
intake now requires the current content-policy grant plus a per-upload
`rights`/`права` acknowledgement. Focused bot, store, presentation and
coordinator suites pass; actor-bound foreign-reference denial, targeted-track
no-fallback and non-leaking capability presentation have automated coverage.
The accepted production codec no-go remains in force, and real Telegram,
recipient and hardware evidence remains deferred to `TASK-260712-3u5cdn`.
Counts remain 109/205 overall and 109/186 engineering until hosted CI and merge
accept this task.

Checkpoint 2026-07-16 (accepted): `TASK-260712-wt2n7m` landed on exact
engineering head `3a822a1766b80cd0e0f3d67f68b4be3686037af7` through PR #160,
merge `35f5fd4d13267199f3383ee437f5f1fe77bace36`, after hosted run
`29489910594` passed coordinator, node-core, pulsar-win and signed packaged
probe 4/4. Telegram now shares opaque target capabilities, immutable N-target
snapshots, current-Air/include-origin policy, versioned consent, per-upload
rights acknowledgement and non-leaking unsupported-target presentation with
the app path. The additive callback companion makes old-binary rollback fail
closed. Targeted queue/replace stays honestly unsupported under the accepted
production codec no-go; real Telegram, recipient, packaged-app and audible
evidence remains manual in `TASK-260712-3u5cdn`. Progress is 110/205 overall
and 110/186 engineering; strict execution advances to `TASK-260712-3lximx`.

Checkpoint 2026-07-16 (accepted): `TASK-260712-3lximx` landed on exact
engineering head `2598c2af882ff589bc1ca0431bf1c6f708253f99` through PR #162,
merge `c1a909652cab82807dc483ee3dd4afdf1c2b7416`, after hosted run
`29491811217` passed coordinator, node-core, pulsar-win and signed packaged
probe 4/4. Windows now has crash-safe app-private long-track drafts, bounded
64 KiB brokered intake, 4 MiB authenticated offset-resumable upload, stable
idempotent retry and a native EN/RU shared-model surface for consent, progress,
targets and playback intent. Queue/replace/pause/seek/resume remain visibly
disabled under the production codec no-go instead of falling back to clips.
Automated keyboard, accessible-label and 96/144/192 DPI evidence is accepted;
real Narrator, packaged MSIX, one-hour audible playback and hardware remain
manual in `TASK-260712-1fpb9q`. Progress is 111/205 overall and 111/186
engineering; strict execution advances to `TASK-260712-2psvhu`.

Checkpoint 2026-07-16 (accepted): `TASK-260712-2ubzyf` landed on exact
engineering head `220ad213b612a6da343eb4f8f1fc3c02ca3c2005` through PR #166,
merge `76d054d8ef8e8195ef3cfad32fcfbe01f4354b53`. The canonical human and
machine-readable handoff freezes the candidate-only variant matrix, bounded
cache/transport/quota values, exact operator metrics, mixed-version policy,
revocation and additive drain-before-rollback order. The accepted codec/player
no-go remains authoritative: current artifacts can dark-deploy stages 1-4 but
cannot advertise or activate `stream_track_v1`; stages 5-8 require a reviewed
replacement ADR, explicit registries and additive policy-schema revision.
Contract tests passed 49/49. Hosted run `29494894143` attempt 2 passed all four
jobs after an initial dependency-download failure and Windows TempDir cleanup
flake (coordinator 2m16s, node-core 1m51s, pulsar-win 1m48s, signed packaged
probe 2m28s). Real app, hardware, audible, production-shaped rollback and beta
evidence remains manual and unclaimed in `EPIC-260714-th54l3`. Progress is
113/205 overall and 113/186 engineering; strict execution advances to
`TASK-260712-14rxuk`.

Checkpoint 2026-07-16 (accepted): `TASK-260712-14rxuk` landed on exact
engineering head `12c300c` through PR #168, merge
`d3db8c9f367bc5de2fd40bd047050100ddcc1825`. The shared
`p2-gate-matrix-evidence.v1` contract freezes all B1-B7, sections 17-18, 20.5
and 20.6 gates, exact source hashes, 3+30 timing samples, monotonic clocks,
Windows-Windows/Windows-macOS/macOS-macOS pairings, 3x5 real and 8x20
synthetic Air shapes, fixture locks, provenance, artifact naming, privacy
denylist and critical-incident/beta reset rules. Contract and campaign
validators fail closed on threshold, source, claim-class, package, clock,
sanitization and artifact-hash drift. Sixty contract tests and the Air
regression harness passed; hosted run `29496295085` passed all four jobs
(coordinator 2m26s, node-core 1m28s, pulsar-win 1m28s, signed packaged probe
2m46s). No manual result is claimed: codec/player no-go, hardware, participants,
production credentials, rollback and seven-day beta remain explicit blockers
in `EPIC-260714-th54l3`. Progress is 114/205 overall and 114/186 engineering;
strict execution advances to `TASK-260712-2g3fkt`.

Checkpoint 2026-07-16 (blocked, not accepted): the source-linked engineering
review for `TASK-260712-2g3fkt` landed on exact head `87f5851` through PR #170,
merge `affa66ab830696e38e923f217a3b43dd5e95b581`. It pins the rubric, license
audit, comparative matrix and player handoff; reruns eight validators, 19
codec tests, pure-Go and macOS native probe builds, Go module verification,
race tests and current called-symbol vulnerability scanning; and adds a
fail-closed machine-readable review validator with five negative tests. Hosted
run `29497274813` passed all four jobs (coordinator 2m18s, node-core 1m11s,
pulsar-win 1m53s, packaged probe 2m38s). The outcome is intentionally not an
independent acceptance: the same root session implemented earlier codec work.
No exact Windows-plus-macOS combination passes every hard gate, six High
findings remain open, production playback is forbidden and legal/hardware
claims remain unmade. The board task is `blocked`; checklist items for reviewer
independence and High-finding fix/re-review remain open. Progress therefore
stays 114/205 overall and 114/186 engineering, and strict execution does not
advance to `TASK-260712-28mn7w`.

Checkpoint 2026-07-16 (accepted fail-closed outcome): the owner continuation
rule resolves the sequencing treatment of `TASK-260712-2g3fkt`. Its acceptance
criterion explicitly permits a source-linked report that blocks Phase 2, so
the landed `BLOCK PHASE 2` review is accepted as the engineering task outcome,
without converting any failed gate into a pass. Qualified AAC/LGPL counsel,
implementation-independent review and exact candidate release proof were
transferred to `TASK-260716-tlxe3s` — approve-p2-codec-candidate-legal-supply-gates
under `EPIC-260714-zmnd4n` — Critical owner approval questions. The proposed
default is bundled FFmpeg 8.1.2 minimal shared, package-local, no CLI, no
runtime download and dark-only; it is not legal advice or production
authorization. Progress is 115/205 overall and 115/186 engineering. Strict
reversible execution advances to `TASK-260712-28mn7w` while production playback
and Phase 2 promotion remain fail-closed.

Checkpoint 2026-07-16 (accepted fail-closed technical review):
`TASK-260712-28mn7w` landed on exact engineering head
`2b519658390168f6d7b5cffb1b6097cd2e47d077` through PR #173, merge
`8db5c54a745cfc8acbe7975fbe6999b838ffc5d1`. The review fixed the hosted
Windows shutdown High by joining decoder/cache cleanup in `Close`; the new
deterministic regression and former cleanup-flake path passed 100 repetitions.
Full Windows and relevant coordinator suites passed under race detection, six
macOS candidate-player tests and 38 stream contracts passed, and hosted run
`29499587834` passed all four jobs on its first attempt (coordinator 2m53s,
node-core 1m20s, pulsar-win 2m08s, signed packaged probe 4m24s). Production
remains blocked by the bounded whole-object integrity gap, physical p95/RSS/
one-hour/two-hour matrix and independent review. Those gates are routed to
`TASK-260716-tlxe3s`, manual tasks `TASK-260712-1fpb9q` and
`TASK-260712-2bdi4a`, and independent approval `TASK-260716-3voo6j` owned by
Ivan Oparin. Progress is 116/205 overall and 116/186 engineering. Strict
reversible execution advances to `TASK-260712-2sicfs`.

Checkpoint 2026-07-16 (accepted fail-closed technical review):
`TASK-260712-2sicfs` landed on exact engineering head
`e8d4dc97d823a362c527f429b67f0052fb9698ed` through PR #175, merge
`22f21b882627f359827a804f8bb22b6a9a42f9d2`. The review fixed a High invite
admission bypass: actor and source-IP failure budgets are now reserved before
the store mutation, unavailable guesses retain reservations, and successful
or non-guess outcomes release the exact concurrent reservation. Five invalid
codes followed by a valid sixth can no longer consume before `429`; targeted
race and 100-repeat tests pass. A Medium database scan-error masking defect was
also fixed. The deterministic 8x20 Air rehearsal reports one runtime, no
legacy group and no duplicates; relevant store/runtime/alias/Telegram race,
exact pinned previous-coordinator rollback and 100-repeat transactional-winner
tests passed. Seventy-seven contract tests passed. Hosted run `29501451845`
passed all four jobs on its first attempt (coordinator 2m53s, node-core 2m02s,
pulsar-win 2m09s, signed packaged probe 2m37s). Production Air and Phase 2
promotion remain blocked on manual `TASK-260712-21kz3b` and
`TASK-260712-3qybi2` plus independent approval `TASK-260716-19g4gd` owned by
Ivan Oparin. Progress is 117/205 overall and 117/186 engineering. Strict
reversible execution advances to `TASK-260712-n11rg6`.

Checkpoint 2026-07-16 (accepted fail-closed technical review):
`TASK-260712-n11rg6` landed on exact engineering head
`b18e4dccd92d8adf916349d64592a79242f4c8e0` through PR #177, merge
`70073dbe9fd3f0668d61a4ddb1e8cc23e09c9b1d`. The review fixed a High
consent-integrity defect by rejecting duplicate JSON fields before a durable
policy grant can be created, and fixed Medium cursor error classification so
real SQLite faults are no longer masked as expired capabilities. Twelve pinned
source hashes, 81 contract tests, targeted race suites, six 100-repeat
adversarial store scenarios, consent/pagination/range descriptor stress and
exact previous-coordinator rollback passed. Hosted run `29503347438` passed all
four jobs on its first attempt (coordinator 2m56s, node-core 2m36s, pulsar-win
1m56s, signed packaged probe 2m51s). Production target/range activation and
Phase 2 promotion remain blocked on manual `TASK-260712-3u5cdn` and
`TASK-260712-3qybi2` plus independent approval `TASK-260716-2l5j1a` owned by
Ivan Oparin. Progress is 118/205 overall and 118/186 engineering. Strict
reversible execution advances to `TASK-260712-qi81vf`.

Checkpoint 2026-07-16 (accepted Phase 2 observability and quota views):
`TASK-260712-qi81vf` landed on exact engineering head
`b54ccd720f1ec00f372d39645984d143e7c9d892` through PR #179, merge
`347d7ae2e03780f95530748ed59cb90baf391b77`. The authenticated no-store
operator view now aggregates canonical stream accounting, processing,
playback, target/inbox and Air state with a rolling 24-hour window and fixed
privacy-safe dimensions. Public health uses a lightweight snapshot, fails only
enabled Phase 2 dependencies and preserves Phase 1 with flags off. Exact SQL
order-statistic p95 uses constant memory; server timestamps are honestly
identified as wall-clock milliseconds, while client seek/buffer/audible
evidence remains manual and unclaimed. Observability and Air delta validators,
85 contract tests, `go vet`, focused race suites and 100-repeat tests passed.
Hosted run `29506259964` passed all four jobs on its first attempt (coordinator
2m50s, node-core 1m49s, pulsar-win 1m53s, signed packaged probe 2m17s).
Progress is 119/205 overall and 119/186 engineering. Strict reversible
execution advances to `TASK-260712-1kfnpu`.

Checkpoint 2026-07-16 (accepted Phase 2 root integration review):
`TASK-260712-1kfnpu` reviewed the exact source candidate
`5f2f7e97a343b4bca84fe56ee57dd02458265f31` and tree
`4a03b4d3a3db062ed210e6696869366a9b6cf775` across 624 no-rename paths,
50 Phase 2 tasks, 102 first-parent intervals and B1-B7. The engineering
baseline is accepted for reversible continuation, while production build,
package and config hashes remain null, the codec decision remains no-go and
13 High production/manual/external findings remain fail-closed. The review
also made OGG test fixture generation portable to FFmpeg installations without
`libvorbis`; production codec behavior did not change. Clean local acceptance
passed all 12 stages and 89 contract tests. Hosted run `29509397804` passed all
four jobs on its first attempt (pulsar-win 2m07s, node-core 2m21s, signed
packaged probe 2m45s, coordinator 3m02s). PR #181 landed commits `5f2f7e9` and
`7287258` at merge `a1c4d08988624d3ba5d9c2e6834541bfee879d92`.
Progress is 120/205 overall and 120/186 engineering. Strict reversible
execution advances to `TASK-260712-3a0cf9`.

Checkpoint 2026-07-16 (accepted Phase 2 engineering handoff):
`TASK-260712-3a0cf9` published a single reproducible Phase 2 index on exact
baseline `cfb6fa3801742e1150ca22d95452093efe2c037d`. It source-pins 27
contracts, reviews, runbooks, authorities and tools; maps B1-B7, 20.5,
section 18 and 20.6; records quota defaults, feature authority, rollback
ownership, all six Phase 2 manual tasks and four external approvals. Production
build/package/config/database/fixture hashes remain null, codec remains no-go,
13 High production gates remain open and rollout remains capped at dark stage
4. Clean local acceptance passed all 12 stages with 93 contract tests; the
synthetic Air 8x20 rehearsal had zero failures or duplicate commands. Hosted
run `29511154644` passed all four jobs on its first attempt (pulsar-win 1m57s,
node-core 2m01s, coordinator 2m18s, signed packaged probe 2m29s). PR #183
landed exact head `fa03a479388ffd41031637b521d8de0eb71f89e9` at merge
`b02538f201cdfe40fd4bbfb5150842fd96754861`. This opens reversible P3
engineering only. Progress is 121/205 overall and 121/186 engineering. Manual
`TASK-260712-9wivva` stays deferred; strict engineering advances to
`TASK-260712-lo7a68`.

Checkpoint 2026-07-16 (accepted generation-safe live PTT wire contract):
`TASK-260712-3qviqc` freezes eight additive `live_ptt_v1` JSON signals, a
40-byte big-endian binary Opus envelope, random non-zero 128-bit session IDs,
monotonic generation and sequence guards, frozen targets, no late join and
non-resume after disconnect or restart. Exact duplicates are idempotent;
stale, oversized, truncated, wrong-profile, unauthorized and post-terminal
input fails closed. Capture authority is local-user-input-only, unsupported
targets receive explicit terminal receipts, and there is no hidden clip
fallback. Go coordinator and Windows remain byte-equal; Swift is independently
typed; 59 signalling goldens and binary start/middle/end plus malformed vectors
are shared. The capability remains unadvertised until later runtime and platform
tasks pass, while physical C2 evidence stays manual in `TASK-260712-1rzqh9`.
Clean repository acceptance passed all 12 stages and hosted run `29515367395`
passed all four jobs. PR #187 landed exact code head `667f43904fb9afe0c346966f47b0ed56a10d1890`
at merge `4ec46a121c4da7dfc702491eb8ab296edfb8763a`. Progress is 123/205 overall
and 123/186 engineering. Strict execution advances to `TASK-260712-3vzbbl`.

Checkpoint 2026-07-16 (accepted bounded ephemeral coordinator live PTT runtime):
`TASK-260712-3vzbbl` adds an env-dark coordinator runtime that authenticates
the sender socket, resolves current Air/barycenter policy, seals exact targets
and replaces caller identity with a random session ID and monotonic generation.
Validated binary frames use ordered fixed per-connection queues, isolated
non-blocking target backpressure, a 50 frame/s token bucket, global/session
bounds and metadata-only health; no audio bytes enter storage or ordinary
logs. Policy is rechecked before accept and continuously, targets never expand,
reconnect/restart/watchdogs terminate without resume, and live duck/release is
serialized with durable overlay/interrupt work. The capability remains off and
unadvertised; physical latency, recovery and real-hardware audio evidence stays
manual in `TASK-260712-1rzqh9`. Clean repository acceptance passed all 12
stages and hosted run `29518925339` passed all four jobs. PR #189 landed exact
code head `ca75072d6805442bb2ef31afba33201a0827e8b2` at merge
`81fdb940574d13221909f31226380c8e1a9034ed`. Progress is 124/205 overall and
124/186 engineering. Strict execution advances to `TASK-260712-19w1qn`.

Checkpoint 2026-07-16 (accepted bounded macOS live jitter receiver):
`TASK-260712-19w1qn` adds a single-session nine-packet jitter window, a fixed
320 ms PCM ring and a 48 kHz mono render source mixed through the existing
post-mix limiter and local master gain. Start validation rejects unauthorized,
stale-generation, malformed-profile and concurrent sessions before decode;
sequence, timestamp, duplicate, conflict and frozen-window checks fail closed.
Decoder and timer work stay on a serial control queue. The self-contained
macOS backend decodes raw Opus through `AVAudioConverter`; a reviewed backend
may supply one-frame FEC, while the system path explicitly falls back to
bounded attenuated waveform PLC and stops after eight consecutive
concealments. Pre-duck, live gain, drain, cancel, timeout and replacement-safe
cleanup use ramps and generation tokens. No audio is persisted and the
capability remains unadvertised. Deterministic 2% loss proves bounded state and
continuity only; physical intelligibility, audible PLC/click quality and
calibrated latency remain manual in `TASK-260712-1rzqh9`. Full Swift passed
263/263, focused receiver coverage passed 8/8, clean repository acceptance
passed 12/12 and hosted run `29521325367` passed all four jobs. PR #191 landed
exact code head `c4a8fd1` at merge
`9c1d0c2a4e3fc2bb0f339ccef57945ac5ffa4f4c`. Progress is 125/205 overall and
125/186 engineering. Strict execution advances to `TASK-260712-26mnp1`.

Checkpoint 2026-07-16 (accepted bounded macOS live capture sender):
`TASK-260712-26mnp1` binds microphone capture to one current local hold
generation plus the matching authorized coordinator start; an unsolicited or
stale start cannot open or resume capture, and unavailable release-capable
input falls back to the existing clip path before microphone access. A fixed
3,840-sample mailbox keeps encoder, framing, transport and metering off the
capture callback. Fixed 20 ms raw Opus frames use one-frame lookbehind and an
eight-frame outbound bound. Release, Stop, watchdog, duration, sleep, lock,
permission revoke, device loss, quit, disconnect, overflow and backpressure
converge on generation-safe teardown with no live-media persistence. The
self-contained Apple encoder is engineering-only because its API does not
expose the frozen libopus FEC and complexity controls; `live_ptt_v1` therefore
remains unadvertised. Physical hold, microphone/device/sleep/lock behavior,
audible cues and real-hardware cycles remain manual in `TASK-260712-1rzqh9`.
Focused sender coverage passed 7/7 including 100 deterministic cycles, full
Swift passed 273/273, clean repository acceptance passed 12/12 and hosted run
`29523191600` passed all four jobs. PR #193 landed exact code head `d5868f9`
at merge `eac1c183144df93ea126c9c595bb6dca8a8cd842`. Progress is 126/205
overall and 126/186 engineering. Strict execution advances to
`TASK-260712-1ckdr7`.

Checkpoint 2026-07-16 (accepted bounded Windows live jitter receiver):
`TASK-260712-1ckdr7` adds a single-session nine-packet reorder window, fixed
320 ms PCM ring and an allocation-free render branch mixed through the common
-1 dBFS limiter and local master gain. Start validation rejects unauthorized,
stale-generation, malformed-profile and concurrent sessions before decode;
sequence, exact-timestamp, duplicate, conflict, late and frozen-window checks
fail closed. Decode, FEC/PLC and timer work stay off WASAPI render. The
production decoder is intentionally not registered because the accepted codec
spike found no reviewed signed Windows libopus supply path; the capability
therefore stays unadvertised. Deterministic 2% loss proves bounded state,
continuity and cleanup only. Signed-app playback, physical intelligibility,
latency, audible PLC/click quality and real-hardware lifecycle evidence remain
manual in `TASK-260712-1rzqh9`. Focused receiver coverage passed 6/6, full Go
test and race suites, amd64/arm64 Windows cross-builds and clean repository
acceptance 12/12 passed; hosted run `29525402024` passed all four jobs. PR #195
landed exact code head `987de8b` at merge
`365fb117e04d2bb8f462b7cd3bd29b7339d797a5`. Progress is 127/205 overall and
127/186 engineering. Strict execution advances to `TASK-260712-ezdhpf`.

Checkpoint 2026-07-16 (accepted bounded Windows live capture sender):
`TASK-260712-ezdhpf` binds the Phase 1 AppCapability/WASAPI backend to one
current local hold generation and a matching authorized coordinator start.
Unsolicited, unauthorized and stale starts cannot request permission or open
the microphone; unavailable hold input falls back before capture. Fixed worker
buffers normalize 8--192 kHz input into exact 20 ms mono frames, while an
eight-frame transport queue keeps network work off the capture worker. Release,
lost release, local Stop, lock, suspend, permission/device loss, disconnect,
quit, maximum duration, encoder failure and backpressure converge on stream
stop/close and generation-safe cleanup. Root review additionally kept the
sender in `stopping` until the ordered terminal frame/control drain completes,
preventing overlap with a new hold. No live media is persisted or logged. The
production encoder and `live_ptt_v1` advertisement remain absent under the
accepted signed-libopus no-go. Focused coverage passed 8/8 and ten repeated
runs; full Go test/race, vet, amd64/arm64 Windows cross-builds and clean
repository acceptance 12/12 passed. Hosted run `29527709243` passed all four
jobs. PR #197 landed exact code head `c3a89d0` at merge
`6d569e3216fd6fe72be9c683e299ddcfa10e6fa4`. Signed Windows 10/11 real-app
stress remains explicitly unpassed here and manual in `TASK-260712-1rzqh9`.
Progress is 128/205 overall and 128/186 engineering. Strict execution advances
to `TASK-260712-2kj9kj`.

Checkpoint 2026-07-16 (accepted production-dark macOS live PTT node):
`TASK-260712-2kj9kj` composes the generation-bound macOS sender, bounded jitter
receiver/mixer and frozen signalling behind injected authoritative target and
incoming DND/policy decisions. Capture begins only after a matching validated
accept, stale or concurrent directions fail closed, and button/menu/shortcut
hold seams fall back to the existing clip path when release-capable input is
not proven. The coordinator client now exposes an authenticated,
capability-gated binary `BP` seam with eight sends in flight, while shipping
registration and app construction deliberately remain absent. Release, Stop,
remote terminal state, sleep, lock, disconnect, permission recheck, rollback
and quit converge on generation-safe sender/receiver cleanup. Exact terminal
vocabulary is aligned with the Go contract and validated before send. Focused
Swift coverage passed 24/24, the full package passed 280/280, and clean
repository acceptance passed 12/12. A TSan-instrumented build completed, but
the host rejected the Xcode sanitizer dylib signature at runtime; no race-free
sanitizer claim is made. Hosted run `29529995520` passed all four jobs. PR #199
landed exact code head `e7472b2` at merge
`f33f1fbb8330ce946e5ecf748f7a522d2ba32d81`. No global key-down/up,
Accessibility request, audible or real-hardware result is claimed; those stay
manual in `TASK-260712-1rzqh9`. Progress is 129/205 overall and 129/186
engineering. Strict execution advances to `TASK-260712-2jbo5i`.

Checkpoint 2026-07-16 (accepted production-dark Windows live PTT node):
`TASK-260712-2jbo5i` composes the generation-bound Windows sender, bounded
jitter receiver/mixer and frozen signalling behind injected authoritative
target and incoming DND/policy decisions. Atomic direction claims prevent a
racing local hold and incoming start from opening capture and playback
together. The WebSocket client now uses one connection-bound FIFO for validated
live controls and binary frames: eight binary and 16 control slots are bounded,
Start/frame/End order is preserved, malformed or unadvertised traffic fails
closed and items from an old socket are discarded after reconnect. Shipping
registration and app construction remain absent. Release, Stop, remote
terminal state, lock, suspend, permission/device loss, disconnect, rollback
and quit converge on generation-safe cleanup. Focused live coverage passed ten
repeated runs, full Go vet/test/race and Windows amd64/arm64 CGO-free
cross-builds passed, and clean repository acceptance passed 12/12. Hosted run
`29532276399` passed all four jobs. PR #201 landed exact code head `100e447` at
merge `b4fb6f7abdf0f4f669b123afb9f3a136a0161efb`. `RegisterHotKey` remains a
toggle-only `WM_HOTKEY`; no AppContainer global key-down/up, `SetWindowsHookEx`,
audible or physical-hardware result is claimed. Those checks stay manual in
`TASK-260712-1rzqh9`. Progress is 130/205 overall and 130/186 engineering.
Strict execution advances to `TASK-260712-3sj8ox`.

Checkpoint 2026-07-16 (accepted automation safety contract):
`TASK-260712-3sj8ox` resolves the deferred surface decision in favor of the
coordinator's authenticated HTTPS `POST /v1/automation/triggers`; v1 adds no
loopback listener, webhook, callback URL or server-side fetch. Automation is
limited to durable ready `audio_clip` cues and hash-pinned builtins, uses only
overlay delivery, and explicitly rejects microphone/voice, tracks, live PTT,
arbitrary media and silent fallback. The contract freezes least-privilege
one-time scoped principals, immutable cue/target scope, `own_barycenter`, exact
bound-Air and allowlisted explicit audiences, no `this_pulsar` DND exception,
block/DND/quiet-hour precedence, IANA timezone plus DST/no-catch-up occurrence
identity, rate/concurrency caps, revoke/disable races, audit attribution and
fail-closed `automation_cue_v1` mixed-version behavior. Executable Go constants,
pure admission ordering and document guards keep the contract production-dark.
Coordinator vet/full tests and automation race x10 passed; Windows full tests
passed after one unrelated transient capture-workflow failure was repeated
successfully 20 times, and the clean exact-head acceptance suite passed 12/12
with `manualEvidence=not-run`. Hosted run `29533919029` passed all four jobs.
PR #203 landed exact code head `c9b8e55` at merge
`4d2fa559b5ceb818ff239e36495c53bc5f841b30`. Real scheduled playback,
packaged controls, local-volume and physical DST/disable evidence remain in
`EPIC-260714-th54l3`. Progress is 131/205 overall and 131/186 engineering.
Strict execution advances to `TASK-260712-hb5xz2`.

Checkpoint 2026-07-16 (accepted saved-cue media lifecycle):
`TASK-260712-hb5xz2` adds owner-scoped, generation-safe durable references to
same-orbit canonical ready app `audio_clip` media and the exact hash-pinned
recording builtin. Active rows are explicit retention pins; derived COUNT/SUM
accounting enforces 64-cue, 50 MiB orbit and 10 MiB/60-second item bounds
without crash-prone counters. Replace, delete and canonical media/actor/orbit
disable transactions write exact-generation `cancel`, `fade_stop` and
`resume_once` revocations, while startup reconciliation fails stale authority
or corrupt sources closed. Focused lifecycle tests passed ten repeats and race
three repeats; clean exact-head acceptance passed 12/12 with
`manualEvidence=not-run`, and hosted run `29536161963` passed all four jobs.
PR #205 landed exact code head `8ccd770` at merge
`ae1812f3a5b6dff20c696a0ef19342a3c38ba83e`. No HTTP route, scheduler,
desktop composition or automation capability was enabled; real-app and
hardware evidence remains in `EPIC-260714-th54l3`. Progress is 132/205 overall
and 132/186 engineering. Strict execution advances to `TASK-260712-3sv87k`.

Checkpoint 2026-07-16 (accepted automation schema and execution lineage):
`TASK-260712-3sv87k` adds production-dark additive SQLite state for feature and
emergency-disable policy, IANA schedules, scoped expiring principals and
immutable schedule/API execution lineage. Unique occurrence and idempotency
identities, fail-closed authority/revoke checks, hashed worker leases and
startup reconciliation preserve at-most-once rows across DST folds, clock
jumps, overlapping workers and crashes. Principals return 32 random bytes once
and persist only a versioned domain-separated digest. Focused tests passed ten
repeats and race three repeats; exact previous-head rollback preserved legacy
media and new automation rows; clean exact-head acceptance passed 12/12 with
`manualEvidence=not-run`. Hosted run `29538458103` passed all four jobs. PR
#207 landed exact code head `6f772ba` at merge `4cd9a30`. No HTTP route,
scheduler loop, target resolution, client composition or production capability
was enabled; real-app and hardware evidence remains in `EPIC-260714-th54l3`.
Progress is 133/205 overall and 133/186 engineering. Strict execution advances
to `TASK-260712-1kk8bd`.

Checkpoint 2026-07-17 (accepted cue, schedule and scoped-principal controls):
`TASK-260712-1kk8bd` adds same-orbit saved-cue CRUD/order, full feature and
schedule controls, and immutable scoped-principal issue/list/revoke APIs. Every
writer reauthorizes the current primary and commits route-bound idempotency
digests plus sanitized replay state atomically. IANA/quiet policy validation,
CAS, canonical generation-fenced target digests, disarmed schedule creation,
one-time 256-bit secrets and immediate revoke fail closed; raw target
capabilities and secret material do not enter replay rows or logs. The scoped
trigger remains a generic production `404` until the downstream runtime is
composed. Focused tests passed ten repeats and race three repeats; full Go
vet/tests/build and exact previous-head rollback passed. Clean exact-head
acceptance passed 12/12 with `manualEvidence=not-run`; hosted run
`29541407173` passed all four jobs. PR #209 landed exact code head `5722332` at
merge `59fa34dde5ae6a515e786b15bce5a468380d46ed`. No scheduler, transmission
dispatch, client composition, real-app or hardware result is claimed. Progress
is 134/205 overall and 134/186 engineering. Strict execution advances to
`TASK-260712-1eva0y`.

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
- [x] `TASK-260712-1sae4q` — media-delete-retention-cleanup (accepted and
  landed: atomic non-disclosing delete, durable expiry/cleanup and frozen
  cancellation seam, exact predecessor rollback, full local race plus hosted
  CI runs `29304495443` / `29304654747` green; PR #14, merge `fe8e73c`)
- [x] `TASK-260712-3mcof4` — media-download-target-acl (accepted and landed:
  owner and
  immutable-target generic GET ACL, live authorization held through descriptor
  acquisition, uniform non-disclosure and isolated legacy bridge; full local
  race plus hosted CI runs `29305916096` / `29306064452` green; PR #15, merge
  `0f3148a`)
- [x] `TASK-260712-12ojcb` — telegram-submitmedia-compat (accepted and landed:
  atomic common-plus-legacy Telegram acceptance, physically bounded source
  download through singleton SubmitMedia, acceptance FIFO and exact reply/
  personal-broadcast/legacy playback parity; full local race, focused 20x
  stress and hosted CI runs `29307473249` / `29307610519` green; PR #16, merge
  `0d6863c`)
- [x] `TASK-260712-gj0cko` — media-acl-delete-retention (accepted and landed:
  integrated generic/legacy authority, rooted retry-safe byte cleanup, durable
  scheduler cancellation and stale-work guards on both clients; full local
  coordinator and Windows race suites, pinned rollback suite, macOS compile,
  and hosted CI runs `29309915183` / `29310143986` green; PR #17, merge
  `9f2aea8`)
- [x] `TASK-260712-3huupe` — media-ingest-acceptance-tests (accepted and
  landed: common-service eight-variant live codec matrix, compact duration-bomb
  rejection, adversarial failure matrix, resumable HTTP-to-ACL/delete/cleanup
  path and real SQLite cleanup restart; full local race, focused 20x stress,
  pinned rollback and hosted CI runs `29311147090` / `29311329355` green;
  PR #18, merge `cfe12ed`)
- [x] `TASK-260712-jolzhh` — media-ingest-docs-handoff (accepted and landed:
  authoritative retry/state/retention/compatibility and cross-story handoff,
  seven-day generic/Telegram default convergence with explicit override,
  rollout/readiness/rollback instructions, full local coordinator race and
  pinned rollback plus hosted CI runs `29312221521` / `29312378238` green;
  PR #19, merge `c4cb324`)

## 3. P1 transmission protocol and scheduler

Story: `STORY-260712-25lysg` — P1 Transmission protocol and scheduler.

- [x] `TASK-260712-51y5k9` — transmission-contract-clarification
- [x] `TASK-260712-1aprcb` — transmission-store-target-snapshots
- [x] `TASK-260712-1g70av` — clip-transmission-wire-contract
- [x] `TASK-260712-2qpp6w` — transmission-http-resolution
- [x] `TASK-260712-26ip33` — macos-transmission-client-hooks (accepted on exact
  code head `9622e00`: generation-safe authenticated prepare/schedule/cancel,
  typed receipts, durable DND/presence and strict capability gating; hosted CI
  runs `29324261074` and `29324579129` green, with 145 Swift tests on the final
  head; PR #24)
- [x] `TASK-260712-2bbz13` — windows-transmission-client-hooks (accepted on
  exact code head `219306c`: generation-safe authenticated
  prepare/schedule/cancel, typed receipts, durable DND/presence, strict
  capability gating and legacy-path redaction; local vet/test/race,
  cross-build and compatibility gates plus all four hosted jobs in run
  `29326895259` and tracking run `29327302466` green; PR #25, merge
  `0c1e194`)
- [x] `TASK-260712-31vvjt` — overlay-controller-scheduler (accepted on exact
  code head `d0e1b92`: durable domain FIFO/barrier/T, authenticated receipts,
  restart/lifecycle cancellation and exact legacy bridge; local full/race/
  rollback gates and all four hosted jobs in run `29331940948` green; PR #26)
- [x] `TASK-260712-2qc27p` — transmission-regression-coverage (accepted on
  exact code head `c60bd99`: complete story-criterion evidence map, 35-pair
  receipt vocabulary, caller-order/ACL/downgrade/barrier/timer adversarial
  coverage and dual exact rollback; local full/race/shuffle/platform gates and
  all four hosted jobs in run `29333494719` green; PR #27)
- [x] `TASK-260712-2cdjq8` — transmission-rollout-handoff (accepted on exact
  documentation/code head `cd234c9`: one guarded contract/evidence index,
  coordinator-first mixed-version rollout, drain-before-rollback procedure and
  exact downstream semantics; local full/race/platform gates and all four
  hosted jobs in run `29334550550` green; PR #28)

## 4. P1 policy and moderation foundation

Story: `STORY-260712-1tgryz` — P1 Policy and moderation foundation.

- [x] `TASK-260712-16zfvu` — confirm-legal-ops-inputs (accepted on exact head
  `3b12371`: all seven owner-approved legal/operations groups, strict
  machine-readable validation and pre-submit fail-closed gate; local full/race/
  platform gates and all four hosted jobs in run `29338589269` green; PR #29)
- [x] `TASK-260712-2kec2s` — moderation-control-plane (accepted on exact code
  head `2a0b135`: additive least-privilege report/operator/evidence/decision
  control plane with canonical enforcement, deterministic retention,
  migration and rollback coverage; local platform gates and all four hosted
  jobs in run `29342009648` green; PR #30)
- [x] `TASK-260712-g9ycx5` — verify-current-store-policy (accepted on exact
  code head `f0bcace`: official v7.19 requirements/asset matrix, guarded
  finding provenance and fail-closed tag/freshness/delta submit gate; local
  platform gates and all four hosted jobs in run `29343948310` green; PR #31)
- [x] `TASK-260712-1epb3a` — privacy-ugc-policy-pack (accepted on exact code
  head `27c19cd`: versioned EN/RU policy sources, 44-section factual
  traceability, exact-hash parity validator and fail-closed Store publication
  gate; local full/race/platform gates and all four hosted jobs in run
  `29345880750` green; PR #32)
- [x] `TASK-260712-1x0lot` — publish-policy-support-pages (accepted on exact
  engineering head `2da485f`: deterministic exact-hash EN/RU bundle, product
  and Store wiring, explicit clean-path redirects, edge byte-preservation and
  live verification of all 20 routes; all four hosted jobs in run
  `29348947568` green; `pulsar-site` production commit `6322e28`; PR #33)
- [x] `TASK-260712-3t9nr8` — moderation-runbook-mailbox (engineering accepted
  on exact head `9bcce41`: accountable runbook, report-scoped content-free
  audit export, deterministic operations validator and fail-closed Store mail
  gate; all four hosted jobs in run `29350324690` green; real MX/delivery
  remains external `TASK-260714-200ib8`; PR #34)

## 5. P1 cross-platform overlay and interrupt mixer

Story: `STORY-260712-fes2jj` — P1 Cross-platform overlay and interrupt mixer.

- [x] `TASK-260712-1hqiek` — render-safe-clip-state-foundation (accepted on
  exact code head `8521b84`; all four hosted jobs in run `29351870335` green;
  no real-device evidence claimed; PR #35)
- [x] `TASK-260712-1viwvi` — windows-overlay-duck-limiter (accepted on exact
  code head `dac4310`; all four hosted jobs in run `29353275479` green; no
  physical/audible result claimed; PR #36)
- [x] `TASK-260712-2zbmq4` — macos-overlay-duck-limiter (accepted on exact
  code head `731c83d`; all four hosted jobs in run `29354780914` green; no
  physical/audible result claimed; PR #37)
- [x] `TASK-260712-1g6lk8` — windows-interrupt-resume (accepted on exact
  engineering head `a29db30`; all four hosted jobs in run `29356446731`
  green; physical/audible A4 remains manual; PR #38)
- [x] `TASK-260712-8mwyiv` — macos-interrupt-resume (accepted on exact
  engineering head `2a06f2f`; all four hosted jobs in run `29357878003`
  green; physical/audible A4 remains manual; PR #39)
- [x] `TASK-260712-3d6cnn` — overlay-interrupt-regression-tests (accepted on
  exact engineering head `f45de46`; all four hosted jobs in run `29358958855`
  green; physical/audible A3/A4 remains manual; PR #40)
- ↪ manual `TASK-260712-2hodti` — overlay-interrupt-live-evidence →
  `EPIC-260714-th54l3`

## 6. P1 Telegram adapter, history and presence

Story: `STORY-260712-34kbkn` — P1 Telegram adapter, history and presence.

- [x] `TASK-260712-3coble` — phase1-history-presence-contract (accepted on
  exact engineering head `dfefae6`; all four hosted jobs in run `29360209758`
  green; no real-app/hardware result claimed; PR #41)
- [x] `TASK-260712-1gx6mh` — shared-delivery-presentation-model (accepted on
  exact engineering head `31024a2`; all four hosted jobs in run `29361254030`
  green; no real-app/hardware result claimed; PR #42)
- [x] `TASK-260712-3dmllz` — telegram-callback-audio-transport (accepted on
  exact engineering head `773b417`; all four hosted jobs in run `29362994920`
  green; no real Telegram/audible/hardware result claimed; PR #43)
- [x] `TASK-260712-1c1ska` — presence-dnd-block-surface (accepted on exact
  engineering head `a65fc65`; all four hosted jobs in run `29365735642` green;
  no real-app/audible/hardware result claimed; PR #44)
- [x] `TASK-260712-2hcq1g` — transmission-history-receipt-query (accepted on
  exact engineering head `742c160`; all four hosted jobs in run `29368167361`
  green; tracking head `835efb7` passed run `29368383324`; no real-app/Telegram-
  client/audible/hardware result claimed; PR #45, merge `77cf82f`)
- [x] `TASK-260712-21ers7` — telegram-inline-routing-compat (accepted on exact
  engineering head `8fc47cf`; all four hosted jobs in run `29370460972` green;
  tracking head `a9c6def` passed run `29370645888`; no real-app/Telegram-client/
  audible/hardware result claimed; PR #46, merge `912d080`)
- [x] `TASK-260712-3e4p0c` — history-replay-policy-actions (accepted on exact
  engineering head `04f2b20`; all four hosted jobs in run `29372823415` green;
  no real-client/audible/hardware result claimed; PR #47)
- [x] `TASK-260712-3d0zgu` — telegram-parity-regression-tests (accepted on
  exact engineering head `24a043e`; all four hosted jobs in run `29373913897`
  green; no real-app/Telegram-client/audible/hardware result claimed; PR #48)
- [x] `TASK-260712-1f9jtm` — telegram-parity-docs-handoff (accepted on exact
  engineering head `14d3d5a`; all four hosted jobs in run `29374582024` green;
  final story handoff published; no real-client/audible/hardware result
  claimed; PR #49)

## 7. P1 main UI, local self-test and capture

Story: `STORY-260712-2e36uz` — P1 Main UI, local self-test and capture.

- [x] `TASK-260712-1c04pk` — macos-main-window-menubar-shell (accepted on exact
  engineering head `895eddf`; all four hosted jobs in run `29375974503` green;
  no real-app/live-VoiceOver/audible/microphone/hardware result claimed; PR #50)
- [x] `TASK-260712-2lrpc0` — builtin-cue-temp-media-contract
- [x] `TASK-260712-30abcm` — macos-microphone-capture-engine (accepted on exact
  engineering head `18bae35`; all four hosted jobs in run `29379013937` green,
  including 188 Swift tests; no real microphone/audible/hardware result claimed;
  PR #52)
- [x] `TASK-260712-9i5se7` — windows-main-window-tray-shell (accepted on exact
  engineering head `9045097`; all four hosted jobs in run `29380174085` green;
  no real packaged UI/Narrator/DPI hardware result claimed; PR #53)
- [x] `TASK-260712-2w4gyw` — windows-microphone-capture-engine (accepted on
  exact engineering head `b40bd16`; all four hosted jobs in run `29381000568`
  green on rerun, including signed MSIX build/install/cleanup; real microphone,
  permission UI, hidden capture and lifecycle hardware remain manual; PR #54)
- [x] `TASK-260712-3lg0ht` — macos-self-test-file-intake (accepted on exact
  engineering head `50b872d`; all four hosted jobs in run `29382291652` green;
  real microphone, audible route and Finder picker/drop evidence remain manual;
  PR #55)
- [x] `TASK-260712-ut6akw` — macos-hotkey-menubar-recording (accepted on exact
  engineering head `188c30d`; all four hosted jobs in run `29383052378` green,
  including 202 Swift tests and signed MSIX packaging; real shortcut/conflict,
  hidden-window and lifecycle observations remain manual; PR #56)
- [x] `TASK-260712-25at8b` — windows-self-test-file-intake (accepted on exact
  engineering head `88868cc`; all four hosted jobs in run `29384112933` green,
  including signed MSIX packaging; real microphone/output, permission UI and
  Explorer picker/drop observations remain manual; PR #57)
- [x] `TASK-260712-c7dmv8` — windows-hotkey-tray-recording (accepted on exact
  engineering head `e70e2ea`; all four hosted jobs in run `29385014150` green
  after a transient Go-module proxy rerun, including signed MSIX
  build/install/cleanup; real shortcut, conflict, tray, microphone and
  lifecycle observations remain manual; PR #58)
- [x] `TASK-260712-1s6h6t` — macos-local-capture-self-test (accepted on exact
  engineering head `f8e9db9`; all four hosted jobs in run `29385946438` green,
  including 205 Swift tests and signed MSIX packaging; real TCC, microphone,
  audible route/cue, Finder, shortcut/conflict and lifecycle observations
  remain manual; PR #59)
- [x] `TASK-260712-1p8ykc` — windows-local-capture-self-test (accepted on exact
  engineering head `d29f391`; all four hosted jobs in run `29387172394` green,
  including native helper tests and reproducible signed MSIX packaging; real
  permission UI, microphone/output, audible loopback, Explorer, shortcut and
  lifecycle observations remain manual; PR #60)
- [x] `TASK-260712-3dqc3l` — macos-ui-data-integration (accepted on exact
  engineering head `04f4c0f`; 211 Swift tests in 35 suites and release build
  passed locally; all four hosted jobs in run `29388582864` green; real app,
  physical audio, hardware and live-outage observations remain manual; PR #61)
- [x] `TASK-260712-2fe5bz` — windows-ui-data-integration (accepted on exact
  engineering head `af961a5`; local Go test/race/vet, Windows amd64 cross-vet
  and cross-build, coordinator test/vet and 211 Swift tests passed; all four
  hosted jobs in run `29390609436` green; real Windows UI, DPAPI prompt,
  physical audio, live outage and hardware observations remain manual; PR #62)
- ↪ manual `TASK-260712-e5mfqj` — cross-platform-ui-verification →
  `EPIC-260714-th54l3`

## 8. P1 Store compliance and engineering readiness

Story: `STORY-260712-1i0doc` — P1 Store compliance and engineering readiness.
This is the engineering stop before P2; it cannot assert manual or Store
acceptance.

- [x] `TASK-260712-1cdoxh` — acceptance-env-gate-repair (accepted on exact
  replacement head `f8ae903`; clean local 12-stage run and all four hosted jobs
  in run `29392625265` passed with empty end-dirty paths; two earlier candidates
  correctly failed closed; real-app/WACK/hardware evidence stays manual; PR #63)
- [x] `TASK-260712-pbfz37` — windows-report-block-delete (accepted on tracking
  head `d5a40c0`; clean seven-stage Windows acceptance pass; all four hosted
  jobs green in run `29393834216`; physical packaged-app keyboard/screen-reader
  evidence remains manual in `TASK-260712-e5mfqj`; PR #64)
- [x] `TASK-260712-34stvx` — macos-report-block-delete (accepted on exact
  engineering head `074e5a7`; clean Swift acceptance passed 215 tests with
  start/end dirty false; all four hosted jobs green in run `29395040109`;
  physical keyboard/VoiceOver evidence remains manual in
  `TASK-260712-e5mfqj`; PR #65)
- [x] `TASK-260712-dlltnr` — telegram-moderation-parity (accepted on exact
  engineering head `8ce1b8c`; clean 12-stage repository acceptance passed with
  start/end dirty false; all four hosted jobs green in run `29397089442` on
  tracking head `88de480`; live Telegram/real-device evidence remains manual
  in `EPIC-260714-th54l3`; PR #66)
- [x] `TASK-260712-e1ie4x` — platform-declarations-localized-copy (accepted on
  exact engineering head `918b377`; clean 12-stage repository acceptance and
  all four hosted jobs passed in run `29398604558`; production EN/RU PRI/MSIX
  schema packed on Windows SDK; actual WACK UI and hardware stay manual; PR #67)
- [x] `TASK-260712-176b74` — p1-independent-protocol-review (technical audit
  and HIGH fix landed through PR #68; external non-implementing signoff is
  tracked in `TASK-260715-3ffm3r`; on 2026-07-19 the reviewer packet was
  refreshed against exact later `main` candidate `191ae263` with a validated
  39-to-59-message delta, exact object hashes and green Go/race plus pinned
  Swift contract suites. Exact packet commit `76e950a` also passed the clean
  coordinator acceptance suite 7/7 with clean start/end and
  `manualEvidence=not-run`; PR #271 merged at `326d60f` after hosted run
  `29684355308` passed 4/4. Owner-authorized independent reviewer Claude Fable
  5 ran through task-board as `RUN-260719-a723c8` at native effort `max`,
  verified the exact candidate and unchanged authority surface through main
  `9e9da97`, reran full coordinator/Windows race suites and 308 Swift tests,
  found no open critical/high issue and recorded APPROVE in
  `TASK-260715-3ffm3r_independent-protocol-review-verdict.md`; repository/CI
  evidence only, with manual/hardware/Store claims still withheld)
- [x] `TASK-260712-1uz0za` — p1-independent-realtime-audio-review (technical
  audit and three HIGH fixes landed through PR #70 at merge `5aedd68`;
  owner-authorized independent reviewer Claude Fable 5 ran through task-board
  as `RUN-260719-3e4ad6`, reviewed later exact main head `11b5132`, inspected
  both render boundaries and the P1-AUDIO-001..003 closures, reran the full
  Windows race suite plus focused soak/leak/memory evidence locally, consumed
  hosted run `29689344361` (4/4 jobs, 308 Swift tests,
  `manualEvidence=not-run`), found no open critical/high engineering issue and
  recorded APPROVE in
  `TASK-260715-s838ym_independent-realtime-audio-review-verdict.md`;
  engineering scope only — manual A3/A4, audible quality and physical
  200/500 ms evidence remain exclusively in `TASK-260712-2hodti`)
- [x] `TASK-260712-1xkn75` — p1-independent-migration-review (initial
  independent run `RUN-260719-d82ed0` found HIGH P1-MIG-003; producer fix
  `831d6d7` moved media reconciliation after dependent schemas and added a
  generation-skip fixture; final independent run `RUN-260719-c83d59`
  approved exact PR head `aafcfc2` after reproducing the old failure and
  passing full race, predecessor 13/13 and hosted CI 4/4)
- [x] `TASK-260712-wy05n6` — p1-independent-security-review (accepted
  2026-07-19 via independent owner-gate signoff `TASK-260715-10ksxz`; Claude
  Fable 5 run `RUN-260719-ca4eaf` verified exact main head `1b9207e`, re-reviewed
  the three closed HIGH findings and eleven trust boundaries, accepted the
  medium dispositions, dispositioned migration MED-1 non-blocking with follow-up
  `BUG-260719-1rsd49`, and confirmed suites/govulncheck/hosted CI `29692957096`
  green)
- [x] `TASK-260712-2s4e9p` — store-listing-iarc-assets (accepted engineering-
  only at exact `e3bf985` by independent Claude Fable 5 run
  `RUN-260719-85bf38`; package/checker/current Microsoft guidance/live URLs
  verified, while manual checklist items 3/4/5/7 remain intentionally false
  and routed to `TASK-260712-e5mfqj` and `TASK-260715-24ube9`)
- [x] `TASK-260712-38lssj` — p1-root-integration-review (accepted as the
  fail-closed engineering root review; exact candidate and unresolved
  independent, Store and manual holds are indexed in
  `docs/analysis/p1-root-integration-review.md`)
- [x] `TASK-260712-1xik11` — p1-engineering-readiness-handoff (accepted through
  PR #80 at merge `9bf3d10` after clean 12/12 and hosted run `29409373973`
  passed 4/4; this authorizes P2 engineering only and does not claim hardware,
  Store submission or independent signoff)

## 9. P2 Air rooms and approach migration

Story: `STORY-260712-3v14m9` — P2 Air rooms and approach migration. This story
is executed before the other canonical Phase 8 story because it is on the epic
critical path.

- [x] `TASK-260712-17yizc` — air-lifecycle-policy-contract (accepted on exact
  head `77fb68231e0c18a1ecb9bdeae5725386d5e64a1a`; clean acceptance 12/12,
  hosted run `29410722718` 4/4, PR #82 merge `b5d10b2`)
- [x] `TASK-260712-3n36ny` — air-schema-link-migration (accepted on exact head
  `b5a633932e7d616bbdee252e1f255c2dfbf49054`; clean acceptance 12/12,
  hosted run `29413065743` 4/4, PR #84 merge `68059d9`)
- [x] `TASK-260712-kr64r2` — air-runtime-session-resolution (accepted on exact
  head `d344f32e20bf1934022acdefc241fbc34a8c0ff9`; clean acceptance 12/12,
  hosted run `29415681872` 4/4 after one unrelated Windows callback retry,
  PR #86 merge `3dcf309`)
- [x] `TASK-260712-2vhf80` — air-control-plane-api (accepted on exact head
  `efa02ac`; clean pinned coordinator acceptance 5/5, hosted run
  `29418360729` 4/4, PR #88 merge `69f32e2`)
- [x] `TASK-260712-25862f` — air-policy-enforcement (accepted on exact head
  `7a3e31f`; full Go tests, vet, targeted race and exact previous-head rollback,
  hosted run `29420598338` 4/4, PR #90 merge `aa40b50`)
- [x] `TASK-260712-2bjdlb` — approach-air-alias-compat (accepted on exact head
  `d2af5aa`; full Go tests/vet, targeted race, exact previous-head Air rollback,
  hosted run `29422446508` 4/4, PR #92 merge `095bf823`)
- [x] `TASK-260712-2i3u7v` — macos-air-room-data-integration (accepted on
  engineering commit `13a65d1`; Xcode Swift tests 221/221, hosted run
  `29424982574` 4/4, PR #94 merge `8cd46b1`)
- [x] `TASK-260712-31zja2` — windows-air-room-data-integration (accepted on
  engineering commit `8b458d8`; exact Windows automated suite 7/7, hosted run
  `29428413069` 4/4, PR #96 merge `203bb1e`)
- [x] `TASK-260712-2zdetx` — telegram-air-lifecycle-parity (accepted on
  engineering commit `e8d8214`; repository automated gate 12/12, hosted run
  `29430796136` 4/4, PR #98 merge `009fba2`)
- [x] `TASK-260712-3nq0tq` — air-lifecycle-regression-rehearsal (accepted on
  engineering commit `b984230`; full coordinator tests/vet/race and repository
  acceptance 12/12, hosted run `29432415158` 4/4, PR #100 merge `e4aa266`)

## 10. P2 codec and streaming player spike

Story: `STORY-260712-3l1r1u` — P2 Codec and streaming player spike.

- [x] `TASK-260712-14u0yk` — freeze-codec-spike-rubric-fixtures-harness
  (accepted on engineering commit `aba592e`; codec tests 10/10 and repository
  acceptance 12/12, hosted run `29434417154` 4/4, PR #102 merge `8f91187`)
- [x] `TASK-260712-dqdoqj` — prototype-stream-variants-range-cache-contract
  (accepted on engineering commit `733b5c6`; codec/stream tests 8/8 and
  repository acceptance 12/12, hosted run `29436698927` 4/4, PR #104 merge
  `f6dd5c2`)
- [x] `TASK-260712-1vdlkw` — audit-codec-licenses-and-distribution-constraints
  (accepted on engineering commit `3fc2409`; codec tests 9/9 and repository
  acceptance 12/12, hosted run `29437923424` attempt 2 passed 4/4, PR #106
  merge `594495b`)
- [x] `TASK-260712-1canzv` — probe-bundled-signed-decoder-path
- [x] `TASK-260712-298tyq` — probe-media-foundation-appcontainer-path
- [x] `TASK-260712-350u8d` — probe-macos-native-streaming-decoder
- [x] `TASK-260712-3vkcki` — probe-pure-go-streaming-decoder-path
- [x] `TASK-260712-ibuaxj` — run-comparative-streaming-evidence-matrix
- [x] `TASK-260712-2eympi` — publish-codec-player-adr-and-handoff

## 11. P2 explicit targets, inbox and transport parity

Story: `STORY-260712-ob1tx2` — P2 Explicit targets, inbox and transport parity.

- [x] `TASK-260712-2rlkp7` — target-inbox-contract-clarification
- [x] `TASK-260712-1c34fe` — common-explicit-target-service
- [x] `TASK-260712-2bk0vy` — target-inbox-store-acl
- [x] `TASK-260712-2ctf3x` — versioned-content-policy-consent
  (accepted on engineering heads `0ff2a72` and `ba19727`; versioned server-owned
  policy grants, RU/EN manifest parity, explicit per-upload rights reminders,
  stale/revoked re-prompting and app/Telegram enforcement are covered without
  treating consent as proof of ownership; hosted run `29459297560` passed 4/4,
  PR #126 merge `c647b2d`)
- [x] `TASK-260712-2j5fkr` — inbox-history-api-pagination
  (accepted on engineering head `3dbf474`; actor-scoped inbox/detail/replay and
  history receipt APIs use stable digest-only cursors, exact current-binding
  authorization, safe presentation, uniform missing behavior and explicit
  idempotent replay; local targeted, race, previous-head, Swift and Windows
  acceptance passed, hosted run `29461136915` passed 4/4, PR #128 merge
  `dbd6baa`)
- [x] `TASK-260712-2zoy4u` — rights-report-disable-enforcement
  (accepted on engineering head `36f51e0`; canonical reporter-local
  revocation covers inbox, replay, direct fetch and future delivery without
  global report-driven censorship; hosted run `29462753677` passed 4/4,
  PR #130 merge `fd6a5df`)
- [x] `TASK-260712-2vipy3` — pulsar-inbox-history-ui
  (accepted on engineering head `5968046`; additive localized target,
  inbox/history/receipt projection and fail-closed Swift/Windows command
  models passed hosted run `29464453352` 4/4; PR #132 merge `45f27ac`)
- [x] `TASK-260712-2nto40` — macos-p2-targets-inbox-ui
  (accepted on engineering head `382e055`; native authenticated SwiftUI
  targeting, consent, inbox/history, receipts and moderation actions passed
  231 Swift tests plus contract/acceptance validators and hosted run
  `29466777419` 4/4; PR #134 merge `22f7175`; hands-on app/hardware checks stay
  in the manual epic)
- [x] `TASK-260712-cuplon` — windows-p2-targets-inbox-ui
  (accepted on engineering head `0b4cd04`; packaged native Win32 explicit
  targets, inbox/history/receipts, moderation and exact durable retry passed
  full/race/vet, amd64/arm64 builds, pinned Windows acceptance and hosted run
  `29468731725` 4/4; PR #136 merge `15f675e`; hands-on app/hardware checks stay
  in the manual epic)
- [x] `TASK-260712-1vklop` — targets-inbox-parity-regressions
  (accepted on engineering head `1b15caf`; 19 fail-closed B5-B7 repository
  invariants and one Windows/macOS/Telegram fixture passed the local all-suite
  matrix and hosted run `29470131117` 4/4; PR #138 merge `029346c`; real app,
  hardware, accessibility, audible and mixed-fleet proof stays in the manual
  epic)
- [x] `TASK-260712-20cuna` — targets-inbox-rollout-handoff
  (accepted on engineering head `43534c8`; versioned coordinator-first deploy,
  mixed-version, rollback and 11-consumer handoff passed 41 contract/unit tests,
  the 12-command local all-suite and hosted run `29470807661` 4/4; PR #140
  merge `e51c937`; B5-B7 and rollout execution remain manual-required)

## 12. P2 streamed user audio tracks

Story: `STORY-260712-2ori1t` — P2 Streamed user audio tracks.

- [x] `TASK-260712-1n5fks` — stream-track-schema-variants
  (accepted on engineering head `b64a671`; additive candidate-neutral variant,
  seek, queue/progress and rollback-safe persistence passed store/full rollback
  coverage and hosted run `29471845396` 4/4; PR #142 merge `5478006`;
  production codec/player remains no-go and hands-on playback stays manual)
- [x] `TASK-260712-31rkpe` — stream-track-wire-contract
  (accepted on engineering head `ea2d6d4`; 51 cross-language goldens,
  generation/timing guards and explicit mixed-version handling passed hosted
  run `29473326227` 4/4; PR #144 merge `0b9fc7d`; production capability
  advertisement and hands-on playback remain disabled)
- [x] `TASK-260712-2ogntd` — stream-storage-egress-accounting
  (accepted on engineering head `00a2697`; actor/orbit storage, processing,
  actual-egress, quota, reconciliation and operator contracts passed local
  full/race/rollback evidence and hosted run `29475162175` 4/4; PR #146 merge
  `15ebd3d`; production traffic and hands-on playback remain unclaimed)
- [x] `TASK-260712-285pag` — audio-track-variant-pipeline
- [x] `TASK-260712-3lf8r0` — stream-range-serving-revocation
  (accepted on engineering head `52bf876`; exact target-generation ACL,
  private single-range GET/HEAD, strong conditionals, uniform revocation,
  symlink-safe immutable opens, actual egress and 1 MiB tiny-range quota floor
  passed full/race/vet/rollback coverage and hosted run `29478459982` 4/4;
  PR #150 merge `cf3a33a`; production codec selection remains locked and
  hands-on playback stays manual)
- [x] `TASK-260712-2h6snp` — streamed-track-coordinator-flow
  (accepted on engineering head `020c9e9`; provider-neutral main-program FSM,
  FIFO queue/replace persistence, exact-generation ready/start/seek fences,
  audible-min progress, rebuffer/restart, Air catch-up/leave and drained-ended
  semantics passed full/race/vet/rollback coverage and hosted run `29480661409`
  4/4; PR #152 merge `d427f82`; production codec/player capability remains
  locked and hands-on playback stays manual)
- [x] `TASK-260712-17w78q` — windows-streamed-track-player
  (accepted on engineering head `a7bfeb7`; authenticated HMAC-keyed bounded
  chunk cache, exact ranges, integrity/ETag/revocation fences, injected decoder,
  fixed 1 MiB PCM ring and generation-safe load/pause/seek/resume/rebuffer/drain
  passed full/race/vet/shuffled/cross-build evidence and hosted run
  `29482823224` 4/4; PR #154 merge `feabd2e`; production stream capability
  remains disabled and real Windows performance stays manual)
- [x] `TASK-260712-1q2kwa` — stream-track-ui-model
  (accepted on engineering head `c6e9a68`; one portable RU/EN draft,
  processing, target-selection and playback-control model, coordinator-owned
  bounded labels, durable resumable draft retention with exact delete echo,
  policy/action/target gates and generation-safe optimistic controls passed
  Windows acceptance 7/7, Swift acceptance 2/2, focused race 20x and hosted
  run `29485664677` 4/4; PR #156 merge `0cb18b9`; production decoder/capability
  and real-app evidence remain disabled and unclaimed)
- [x] `TASK-260712-3aj8w2` — macos-streamed-track-player
  (accepted on engineering head `e6f0685`; authenticated HMAC-keyed bounded
  range cache, injected decoder, fixed 1 MiB PCM ring and generation-safe
  ready/start/pause/seek/resume/progress/rebuffer/drain lifecycle passed 248
  Swift tests and hosted run `29487762262` 4/4; PR #158 merge `6069948`;
  production stream capability stays disabled and real-app evidence stays
  manual)
- [x] `TASK-260712-wt2n7m` — telegram-explicit-target-parity
  (accepted on engineering head `3a822a1`; common opaque Barycenter/Pulsar
  target picker, N-target snapshot, include-origin, versioned consent,
  per-upload rights gate and fail-closed callback rollback passed focused,
  full, race, vet, B5-B7 and hosted run `29489910594` 4/4; PR #160 merge
  `35f5fd4`; production track delivery remains unsupported and real Telegram
  evidence stays manual)
- [x] `TASK-260712-3lximx` — windows-stream-track-ui
  (accepted on engineering head `2598c2a`; crash-safe app-private draft,
  bounded brokered intake, 4 MiB exact-offset upload, stable retry and native
  EN/RU keyboard/DPI-safe controls passed local full/race/vet, Windows cross
  compile and hosted run `29491811217` 4/4; PR #162 merge `c1a9096`; production
  playback and real Narrator/packaged/hardware evidence remain manual)
- [x] `TASK-260712-2psvhu` — macos-stream-track-ui
  (accepted on engineering head `5c977be`; crash-durable 64 KiB app-private
  intake, exact-offset 4 MiB resumable upload, stable reuse semantics,
  per-attempt rights confirmation and EN/RU fail-closed shared-model controls
  passed 254 Swift tests and hosted run `29493756075` 4/4; PR #164 merge
  `533ead1`; production playback remains disabled and real VoiceOver,
  packaged-app, one-hour audible/rebuffer and hardware evidence remain manual)
- ↪ manual `TASK-260712-1fpb9q` — streamed-track-regression-evidence →
  `EPIC-260714-th54l3`
- [x] `TASK-260712-2ubzyf` — streamed-track-rollout-handoff
  (accepted on engineering head `220ad21`; no-go-aware variant/cache/quota,
  dark rollout, metrics, mixed-version, revocation and additive rollback
  contracts passed 49 contract tests and hosted run `29494894143` attempt 2
  4/4; PR #166 merge `76d054d`; activation stays blocked before replacement
  ADR and all real-app/hardware/rollback/beta evidence remains manual)

## 13. P2 engineering integration and rollout readiness

Story: `STORY-260712-1qfbiw` — P2 engineering integration and rollout
readiness. The engineering handoff packet is the stop before P3 development;
manual promotion remains separate.

- [x] `TASK-260712-14rxuk` — phase2-gate-matrix-evidence-contract
  (accepted on engineering head `12c300c`; 17-gate contract freezes source
  hashes, 3+30 samples, monotonic clocks, platform/topology roster, fixture
  locks, artifacts, privacy and beta reset rules; 60 contract tests, Air
  regression and hosted run `29496295085` 4/4 passed; PR #168 merge `d3db8c9`;
  all six real-app/hardware/rollback/beta tasks stay manual and unclaimed)
- [x] `TASK-260712-2g3fkt` — p2-independent-codec-supply-review
  (accepted fail-closed engineering outcome on head `87f5851` through PR #170,
  merge `affa66a`, hosted run `29497274813` 4/4; no codec combination or
  production playback is accepted, and all counsel/independent-review/release
  gates remain preserved in external owner task `TASK-260716-tlxe3s`)
- [x] `TASK-260712-28mn7w` — p2-independent-stream-performance-review — accepted as a fail-closed technical review: Windows shutdown High fixed and re-reviewed; bounded long-track integrity, physical performance evidence and independent signature remain routed production gates
- [x] `TASK-260712-2sicfs` — p2-independent-air-migration-review — accepted as a fail-closed technical review: invite pre-admission High and scan-error masking Medium fixed and re-reviewed; physical rollout and independent signature remain routed production gates
- [x] `TASK-260712-n11rg6` — p2-independent-target-security-review
- [x] `TASK-260712-qi81vf` — phase2-observability-quota-views (accepted on
  exact head `b54ccd7`; canonical privacy-safe operator view, flag-aware
  lightweight health, source-pinned contract/runbook, 85 contract tests,
  focused race and 100-repeat suites passed; PR #179 merge `347d7ae`, hosted
  run `29506259964` 4/4; client/hardware/rollout/beta evidence remains manual)
- [x] `TASK-260712-1kfnpu` — p2-root-integration-review (accepted exact source
  candidate `5f2f7e9` / tree `4a03b4d`; 624 paths and B1-B7 mapped in a
  deterministic manifest; production artifacts remain null, codec no-go and
  13 High production/manual/external holds stay open; clean 12-stage local
  acceptance and hosted run `29509397804` 4/4 passed; PR #181 merge `a1c4d08`)
- ↪ manual `TASK-260712-21kz3b` — phase2-b2-b4-air-scale-acceptance
- ↪ manual `TASK-260712-2bdi4a` — phase2-b1-track-platform-matrix
- ↪ manual `TASK-260712-3qybi2` — phase2-rollout-migration-rollback
- ↪ manual `TASK-260712-3u5cdn` — phase2-b5-b7-rights-mixed-fleet
- ↪ manual `TASK-260712-2pnc5a` — phase2-beta-quota-calibration
- [x] `TASK-260712-3a0cf9` — phase2-engineering-handoff-packet (accepted exact
  head `fa03a47`; 27-anchor B1-B7/20.5/18/20.6 evidence index, flags, quotas,
  rollback and pending manual/external gates frozen; production hashes null,
  codec no-go and rollout stage 4 preserved; clean 12-stage local acceptance,
  93 contract tests and hosted run `29511154644` 4/4 passed; PR #183 merge
  `b02538f`)

## 14. P3 near-live push-to-talk

Story: `STORY-260712-sskhip` — P3 Near-live push-to-talk. This story is
executed before soundboard automation because it is on the epic critical path.

- ↪ manual `TASK-260712-9wivva` — store-safe-hold-input-spike →
  `EPIC-260714-th54l3`
- [x] `TASK-260712-lo7a68` — live-codec-transport-spike (accepted as an
  explicit fail-closed spike on exact engineering head `5cc58e0`; Ivan Oparin
  approved the engineering defaults, and libopus 1.6.1 at 48 kHz mono, 20 ms,
  24 kbit/s constrained VBR, complexity 5, FEC/PLC and a 400-byte payload bound
  is frozen. The local x86_64 macOS benchmark used a 0.010217 real-time factor;
  the deterministic two-leg 2% loss WSS model produced p50 272.266 ms and p95
  458.432 ms while the per-recipient queue stayed bounded. Production remains
  no-go because Windows, macOS arm64, signed-package, hostile-input and physical
  C2/intelligibility evidence are absent. Clean local acceptance passed 12/12
  and hosted run `29512991362` passed 4/4; PR #185 merge `e3f8d63`)
- [x] `TASK-260712-3qviqc` — live-ptt-wire-contract-codec-policy (accepted on
  exact code head `667f439`; clean acceptance 12/12 and hosted run
  `29515367395` 4/4; PR #187 merge `4ec46a1`. The additive signalling and
  binary profile are frozen across Go, Windows and Swift, while the capability
  remains unadvertised and physical C2 evidence remains manual)
- [x] `TASK-260712-3vzbbl` — coordinator-live-ptt-session-runtime (accepted on
  exact code head `ca75072`; clean acceptance 12/12 and hosted run
  `29518925339` 4/4; PR #189 merge `81fdb94`. The runtime is bounded,
  ephemeral, policy-rechecked and serialized with durable overlay/interrupt;
  production capability and physical C2 evidence remain disabled/manual)
- [x] `TASK-260712-19w1qn` — macos-live-jitter-receiver (accepted on exact
  code head `c4a8fd1`; clean acceptance 12/12, Swift 263/263 and hosted run
  `29521325367` 4/4; PR #191 merge `9c1d0c2`. The receiver and render branch
  are bounded and production-dark; physical audio evidence remains manual)
- [x] `TASK-260712-26mnp1` — macos-live-capture-sender (accepted on exact
  code head `d5868f9`; clean acceptance 12/12, focused sender 7/7, Swift
  273/273 and hosted run `29523191600` 4/4; PR #193 merge `eac1c18`. The
  sender is bounded and production-dark; physical lifecycle/audio evidence
  remains manual)
- [x] `TASK-260712-1ckdr7` — windows-live-jitter-receiver (accepted on exact
  code head `987de8b`; focused receiver 6/6, full Go test/race, Windows
  amd64/arm64 cross-builds, clean acceptance 12/12 and hosted run
  `29525402024` 4/4 passed; PR #195 merge `365fb11`. The receiver is bounded
  and production-dark; physical audio evidence remains manual)
- [x] `TASK-260712-ezdhpf` — windows-live-capture-sender (accepted on exact
  code head `c3a89d0`; focused sender 8/8 plus ten repeats, full Go test/race,
  Windows amd64/arm64 cross-builds, clean acceptance 12/12 and hosted run
  `29527709243` 4/4 passed; PR #197 merge `6d569e3`. The sender is bounded and
  production-dark; signed Windows 10/11 hardware stress remains manual)
- [x] `TASK-260712-2kj9kj` — macos-live-ptt-node-integration (accepted on exact
  code head `e7472b2`; focused live tests 24/24, full Swift 280/280, clean
  acceptance 12/12 and hosted run `29529995520` 4/4 passed; PR #199 merge
  `f33f1fb`. Capability and app composition remain dark, and global hold/audio
  hardware evidence remains manual)
- [x] `TASK-260712-2jbo5i` — windows-live-ptt-node-integration (accepted on
  exact code head `100e447`; ten repeated focused runs, full Go vet/test/race,
  amd64/arm64 Windows cross-builds, clean acceptance 12/12 and hosted run
  `29532276399` 4/4 passed; PR #201 merge `b4fb6f7`. Capability/composition stay
  dark, and AppContainer hold/audio hardware evidence remains manual)
- ↪ manual `TASK-260712-1rzqh9` — live-ptt-regression-evidence →
  `EPIC-260714-th54l3`

## 15. P3 soundboard and safe automation

Story: `STORY-260712-326wd5` — P3 Soundboard and safe automation.

- [x] `TASK-260712-3sj8ox` — automation-surface-safety-contract (accepted on
  exact code head `c9b8e55`; executable cue/audience/admission guards,
  coordinator vet/full tests, focused race x10, clean acceptance 12/12 and
  hosted run `29533919029` 4/4 passed; PR #203 merge `4d2fa55`. HTTPS scoped
  API is frozen but production-dark; real-app automation evidence stays manual)
- [x] `TASK-260712-hb5xz2` — saved-cue-media-lifecycle (accepted on exact code
  head `8ccd770`; owner-scoped canonical media pins, derived count/byte quotas,
  generation-safe revoke actions and startup reconciliation; focused tests x10,
  race x3, clean acceptance 12/12 and hosted run `29536161963` 4/4 passed; PR
  #205 merge `ae1812f3`. Automation remains production-dark and manual app/
  hardware evidence remains deferred)
- [x] `TASK-260712-3sv87k` — automation-schema-lineage-foundation (accepted on
  exact code head `6f772ba`; additive schedule/principal/disable/execution
  lineage, fail-closed at-most-once claims, lease recovery and exact previous-
  head rollback; focused tests x10, race x3, clean acceptance 12/12 and hosted
  run `29538458103` 4/4 passed; PR #207 merge `4cd9a30`. Runtime/API/client and
  real-app or hardware evidence remain deferred)
- [x] `TASK-260712-1kk8bd` — cue-schedule-token-control-apis (accepted on exact
  code head `5722332`; same-orbit cue/schedule/feature/scoped-principal control,
  route-bound idempotency, canonical target digests and one-time secrets;
  focused x10, race x3, clean acceptance 12/12 and hosted run `29541407173`
  4/4 passed; PR #209 merge `59fa34d`. Runtime and real-app evidence remain
  downstream/manual)
- [x] `TASK-260712-1eva0y` — automation-runtime-revoke-ratelimits (accepted on
  exact code head `708eaa5`; scoped API and current-minute schedule admission,
  durable idempotency, rolling rate/concurrency limits, fail-closed quiet/Air/
  target policy, deterministic builtin media publication and revoke/disable
  cancellation through the ordinary transmission scheduler; focused race x3,
  exact previous-head rollback, clean acceptance 12/12 and hosted run
  `29543383233` 4/4 passed; PR #211 merge `ca3dda9`. Manual real-app/hardware
  evidence remains deferred)
- [x] `TASK-260712-11e4e3` — automation-history-audit-disable (accepted on
  exact code head `022503c`; canonical history enriches accepted automation
  transmissions and exposes actor-scoped terminal denials, immutable trigger/
  control audit redacts credentials and raw selectors, and history cancel plus
  revision/idempotency-bound schedule disable, principal revoke, and emergency
  disable reuse shared authorities; focused race x3, clean acceptance 12/12
  with 281 Swift tests in 45 suites, and hosted run `29544985523` 4/4 passed;
  PR #213 merge `5e5c41c`. Manual real-app, audible, packaged-interaction, and
  physical-hardware evidence remains deferred)
- [x] `TASK-260712-1yw7fo` — windows-soundboard-hotkeys-schedules (accepted on
  exact code head `1642c57`; canonical manual cue delivery reuses ordinary
  ACL/DND/Air/target/delivery/receipt authority, while the native Windows
  window/tray provide brokered cue CRUD, routing, interrupt confirmation,
  bounded `RegisterHotKey` bindings, honest conflict/button fallback state,
  shared automation history, secret-free preferences and no-capture proofs;
  focused race, clean acceptance 12/12 and hosted run `29547008907` 4/4
  passed; PR #215 merge `47615c4`. Manual signed MSIX, audible output,
  physical keyboard and real-hardware evidence remains deferred)
- [x] `TASK-260712-288j4a` — macos-soundboard-hotkeys-schedules (accepted on
  exact code head `ef77913`; the macOS window and status menu now provide
  stable-ID cue CRUD/order, security-scoped brokered upload, route/delivery,
  canonical manual trigger and interrupt fallback confirmation. Bounded
  exclusive Carbon bindings report recording/OS conflicts, release across
  sleep/session transitions, retain button fallback and persist no token,
  media ID, path or capture state. Display-safe shared automation lineage and
  downstream admin navigation are rendered without taking ownership of the
  next admin task. Clean exact-head pinned Swift acceptance passed 286 tests
  in 46 suites and hosted run `29548357370` passed 4/4; PR #217 merge
  `00df382`. Signed app, audible playback, physical keyboard, prompt and real-
  hardware evidence remains manual in `EPIC-260714-th54l3`)
- [x] `TASK-260712-uht9e2` — telegram-soundboard-automation-parity (accepted on
  exact code head `33b1594`; private `/soundboard` and `/automation` use
  opaque actor/chat/message/revision-bound callbacks, canonical cue/target/
  DND/block/Air/policy/transmission and automation services, schedule next-run
  controls and emergency disable without issuing bearer or automation secrets
  or entering capture. Forged, forwarded, expired, replayed, concurrent and
  role-changed callbacks fail closed; bot prompt downtime leaves desktop state
  unchanged. Exact clean coordinator acceptance passed 5/5 including previous-
  head rollback, focused race and Air/target-security delta reviews passed,
  hosted run `29549870725` passed 4/4; PR #219 merge `b333bd4`. Real Telegram,
  audible app and hardware evidence remains manual in `EPIC-260714-th54l3`)
- [x] `TASK-260712-89fzlc` — windows-automation-admin-ui (accepted on exact
  code head `92443f443f4b012ae56deea839d86009031de1a0`; strict schedule/principal/
  history clients, timezone/DST/quiet-hour editing and projection, CAS and
  idempotency, one-time redacted principal issuance with hardened timed
  clipboard, confirmed destructive actions, emergency disable, pending-history
  cancel and epoch-fenced fail-closed Win32 state/UI. Clean Windows automated
  acceptance `acceptance-92443f4` passed 7/7 with a clean end state and hosted
  run `29551417454` passed 4/4; PR #221 merge
  `1b06463d0ffd441f693dd0c78b5c416c99d6a3cf`. `manualEvidence` is `not-run`;
  real signed-package, accessibility, audible, physical DST and hardware
  evidence remains manual in `EPIC-260714-th54l3`)
- [x] `TASK-260712-1oodka` — macos-automation-admin-ui (accepted on exact code
  head `a83450f4eae625b8f7ae3c54dcb0eac0bb533775`; authenticated fail-closed
  feature/schedule/principal/history client and composition, IANA/DST/quiet-
  hour schedule editing, CAS/idempotency, one-time redacted principal issuance,
  hardened 60-second clipboard lease, confirmed destructive actions, emergency
  disable, pending cancel and display-safe native SwiftUI admin sections. Clean
  exact-head Swift acceptance passed both commands with 291 tests in 47 suites,
  and hosted run `29553117460` passed 4/4; PR #223 merge
  `fa6bc8e3f3908c9bd0abed5efab00613b7ba9476`. `manualEvidence` is `not-run`;
  real signed-app VoiceOver, Full Keyboard Access, screenshots, audible output,
  physical DST and hardware evidence remains manual in `EPIC-260714-th54l3`)
- [x] `TASK-260712-2f0gpu` — automation-safety-evidence-handoff (accepted on
  exact code head `f7f52a56805383f50e0288150c9aaa9feb28fc23`; deterministic
  timezone/DST/quiet-hour, DND/block/Air-policy and principal/orbit concurrency
  regressions; fail-closed machine-readable C7 engineering evidence contract;
  source-hash validator and operator rollback handoff. Clean exact-head all-
  suite acceptance passed 12/12 with `manualEvidence=not-run`; focused race
  passed three times; hosted run `29554336162` passed 4/4; PR #225 merged at
  `793f8aee23ceca1261ae1ba20f0a6988b0f96ffa`. Signed apps, physical clocks,
  hardware, audible/accessibility, live Telegram, recovery and seven-day
  evidence remain unexecuted in `EPIC-260714-th54l3`.)

## 16. P3 end-to-end encrypted media

Story: `STORY-260712-1frfmi` — P3 End-to-end encrypted media. This story is
executed before capture quality because it is on the epic critical path.

- [x] `TASK-260712-2e2ymn` — e2ee-threat-model-claims (accepted on exact code
  head `847a90b8e3fdde89b6d5744d14397bfd11c4d04c`; frozen 22-requirement
  threat/claim contract separates malicious delivery and identity-coordinator
  roles, makes device-set equivocation detectable and claim-blocking, covers
  clips/tracks/cues/live plus Telegram/Spotify exclusions, discloses metadata,
  maps 10 abuse cases and C4-C6, and records external-review entry criteria and
  eight residual risks. Clean exact-head all-suite acceptance passed 12/12
  with `manualEvidence=not-run`; hosted run `29555290473` passed 4/4; PR #227
  merged at `868789cdc828ae6ed08505a35a7e42e9484566d6`. Only the two spikes are
  authorized; implementation, E2EE flag, product claims and independent review
  remain false/not-run.)
- [x] `TASK-260712-16xmy2` — protected-media-container-prep-spike (accepted
  through the task's explicit no-go exit on exact code head
  `00b5bb07d7d97e5e876091fb5030edf328a0151b`; pinned Go 1.25.12
  standard-library `pmc-probe-v1` freezes bounded header/private-manifest,
  chunk AAD, nonce-domain, byte-range and resume-boundary structure plus a
  deterministic vector and fail-closed tamper/truncation/reorder/substitution
  coverage. Clean exact-head acceptance passed 16/16 with
  `manualEvidence=not-run`; hosted run `29556420828` passed 4/4; PR #229
  merged at `478e1aa1c5431e8fdbf443e62afceb5844475dd4`. No production codec,
  container, crypto suite or local preparation toolchain was selected. Signed
  Windows/macOS apps, physical performance, cross-platform Swift vectors,
  whole-container replay state, HTTP range/resume integration, zeroization and
  independent review remain open; E2EE stays blocked, disabled and unclaimed.)
- [x] `TASK-260712-3er89x` — group-crypto-library-spike (accepted through
  the explicit blocking no-go on exact code head
  `7dc56d984b85d09d26036e0afb0271d946b4980c`. RFC 9420 is frozen as the
  only standardized fit candidate, while exact OpenMLS 0.8.1, mls-rs 0.55.2
  and MLS++ snapshots fail the combined audit, binding, Apple-arm64 test,
  Store supply, interop and secret-lifecycle gates. Clean exact-head
  acceptance passed 16/16 with `manualEvidence=not-run`; hosted run
  `29557397257` attempt 1 was cancelled after a proven Windows runner hang,
  and unchanged attempt 2 passed 4/4; PR #231 merged at
  `b3a64badf1232d2273f74af4baa0b6e8f07bbaca`. No library, provider, suite,
  serialization or platform binding is selected; KAT/lifecycle/fork/replay,
  signed-app and independent-review evidence remains not-run, and E2EE stays
  blocked, disabled and unclaimed.)
- [x] `TASK-260712-2ys1ww` — e2ee-protocol-key-lifecycle-contract (accepted
  on exact code `13df61df1c00035d7a1b20674e53bed78c6b394c`. The audit-only
  RFC 9420-semantics authority freezes strict coordinator-visible and commit
  fields, device/Air/target/epoch/generation/sequence/nonce/expiry bindings,
  membership rotation, history grant, recovery/transfer, report-consent and
  no-downgrade rules. Shared content, commit and ten malformed vectors execute
  in coordinator Go, Windows Go and macOS Swift through injected verifier
  seams. Clean exact-head acceptance passed 16/16 with
  `manualEvidence=not-run`; hosted run `29559663767` passed 4/4; PR #233
  merged at `43a4d4e1b6f717a8c36910e8781153d615d43740`. Production suites
  remain empty, `e2ee_media_v1` is not advertised, runtime wiring and
  cryptography are absent, and independent gate `TASK-260712-aniuyy` remains
  mandatory.)

The remaining unchecked E2EE items below are now owned by deferred epic
`EPIC-260716-3qsztl` and keep their internal order there. They are not counted
as accepted and cannot begin before the independent gate, but they no longer
block strict best-effort execution of the current engineering epic, which
continues at section 17.

- [x] `TASK-260712-aniuyy` — e2ee-independent-design-review (independently
  approved by Claude Fable 5 max run `RUN-260719-1bbaa7` on exact packet head
  `7e6c8be`; all twelve hashes and four platform/acceptance test families were
  reproduced, with zero open Critical/High design findings. Three Low and two
  informational follow-ups are recorded in the verdict. Capability activation,
  production crypto selection, signed-app and hardware claims remain blocked;
  protocol-affecting changes require delta review.)
- [x] `TASK-260712-3w1cst` — encrypted-media-schema-epoch-foundation (accepted
  on exact producer commit `b11377ec22e85a95bc0ad17afc8c7c8d79340cda`.
  Eleven additive tables persist only public state, bounded ciphertext and
  audit while the feature row is CHECK-locked off; exact CAS transitions cover
  epoch commits, fork, replay, object finalize/revoke and grants, with legacy
  rollback compatibility and secret/plaintext backup scans. Shared protocol
  deltas close IDR-001/002/003 on coordinator, Windows and macOS. Producer
  full/race/platform/acceptance suites passed. Independent Claude Fable 5 max
  run `RUN-260719-b1df39` reproduced all 13 pins and suites plus 10/10 scratch
  SQLite adversarial constraints, then APPROVED with zero Critical/High/Medium.
  L1 and I1-I4 remain tracked non-blocking follow-ups; production E2EE and all
  EPC/manual/external gates remain open.)
- [x] `TASK-260712-20j5tm` — coordinator-ciphertext-routing-rotation (accepted
  on exact producer commit `e97717bfad6348279430012ecf0ce3de404eae0d`.
  The additive production-dark coordinator serializes client-produced signed
  proposal/commit events, binds exact protocol-actor/device/Air lineage,
  requires rotation for membership and endpoint changes, gates protected
  writes, and recovers durable per-device delivery without creating, unwrapping,
  escrowing or logging secrets. Producer full/race/vet and acceptance 207/207
  passed. Independent Claude Fable 5 max run `RUN-260719-47433f` reproduced
  all 12 evidence pins and suites, then APPROVED with zero open
  Critical/High/Medium findings. L1 multi-cause audit reason fidelity and I1
  only-revoked-device membership semantics are tracked as non-blocking
  downstream notes; production activation and EPC gates remain open.)
- [x] `TASK-260712-1yz5ca` — coordinator-opaque-media-router (accepted on exact
  producer commit `e4488ed2c0335e57910d704cf4bb4119593bbfdd`; independent
  Claude Fable 5 max completion run `RUN-260719-91776a` reproduced all pins,
  full tests/vet, focused race, acceptance 212/212 and previous-head rollback,
  then APPROVED with zero Critical/High/Medium findings. PR #286 passed 4/4
  hosted jobs and merged to main as `3b08b745590d36e17c6daf8ffe7ef8007decc33a`.)
- [x] `TASK-260712-1x9ruo` — macos-e2ee-key-state (accepted on exact producer
  commit `498957eab686a4e6aad0f653813ccfe3d1d3efa6`. Production-dark device-only
  Keychain state uses distinct metadata/signing/agreement/group/grant/cache
  slots with witnesses, exact predecessor epochs and persist-before-ack crash
  recovery. Producer focused 10/10, NodeCore 318/318 and acceptance 217/217
  passed. Independent Claude Fable 5 max run `RUN-260719-20ab4a` reproduced 9/9
  hashes and the full 16/16 automated battery, then APPROVED WITH NON-BLOCKING
  FOLLOW-UPS with zero Critical/High. Cross-process ownership is a hard
  downstream integration DoD; manual and production crypto gates remain open.)
- [x] `TASK-260712-25dzp4` — windows-e2ee-key-state (accepted on exact final
  producer commit `c7c9b0206f61aa98920e8a21db55265fc9543b96`.
  Production-dark current-user DPAPI state uses distinct device metadata,
  signing, agreement, group, grant and bounded cache slots with independent
  witnesses, cross-process share-none serialization and persist-before-ack.
  Producer and detached clean-worktree harnesses passed 16/16 with 222/222
  contract tests. Independent Claude Fable 5 max run `RUN-260719-c050cd`
  re-verified 14/14 hashes and the full battery at the final SHA, then APPROVED
  WITH NON-BLOCKING FOLLOW-UPS with no Critical/High/Medium code finding. Native
  DPAPI/MSIX/NTFS/profile and forensic evidence remains manual.)
- [x] `TASK-260712-2i0w6x` — report-evidence-moderation-export (accepted on
  exact producer commit `66a34edcbdf8c60fe5827041f0809930c46cfc69`.
  Metadata-only reporting creates no consent/evidence rows; a new evidence
  reference requires explicit consent plus exact report/object/device/manifest
  binding and current recipient authorization. Operator access, immutable
  content-free audit, expiry/delete, crash rollback, restart replay and
  canonical opaque deletion/identity disable paths are covered. Producer clean
  harness passed 7/7 with 227/227 contract tests. Independent Claude Fable 5
  max run `RUN-260720-65a670` repeated the exact-SHA tests and ACCEPTED with no
  Critical/High/Medium finding. Runtime/storage adapter and all real-app,
  traffic, provider and physical evidence remain deferred. Hosted CI run
  `29709135019` passed 4/4 and PR #289 merged as `f9fd2ec`.)
- [x] `TASK-260712-1rziyo` — recovery-device-transfer-history-grants (accepted
  on exact producer commit `94e506629c46473bc890575539750b1a993bbc50`.
  Current-epoch transfer and explicit bounded history grants are exact-lineage,
  replay/expiry/revoke/clone fail-closed and content-opaque; lost-device
  revocation atomically records the required group rotation. macOS/Windows
  identity reset and expired-grant cleanup remain production-dark. Producer
  and independent reviewer full harnesses passed 16/16; Claude Fable 5 max run
  `RUN-260720-6193e1` ACCEPTED with no Critical/High/Medium finding. Real
  devices, native Keychain/DPAPI, signed packages and production crypto remain
  manual/deferred. Hosted CI run `29710412021` passed 4/4 and PR #290 merged
  as `375dc1b`.)
- [x] `TASK-260712-2kcduo` — macos-protected-media-send (accepted on exact
  producer commit `30d23def4350aab22a19824c1e0cbcfad1a5f8da`. The production-dark
  macOS actor reserves a witnessed generation before provider sealing, binds
  exact recipients/epoch/target, validates unique nonces and authenticated
  artifacts, persists ciphertext-only resumable drafts, and applies explicit
  user-owned versus app-owned plaintext cleanup. It remains absent from the
  app composition root and accepts no audit provider through its public path.
  Producer focused 12/12, full macOS 331/331, acceptance 190/190 and automated
  16/16 passed. Claude Fable 5 max run `RUN-260720-cc3c8d` independently
  ACCEPTED with no Critical/High/Medium finding; all production provider,
  signed-app, real-crypto/codec and hardware claims remain deferred. Hosted CI
  run `29711284259` passed 4/4 and PR #291 merged as `856e8a0f`.)
- [x] `TASK-260712-tcwn44` — macos-protected-media-playback (accepted on exact
  rework commit `8c2676206f3fdb44ed54b9ad6f3dc1c5af5728af`. The dormant macOS
  path authenticates exact route/key state and every bounded ciphertext range
  before decoder access, rechecks expiry/membership/history grants per record,
  preserves durable revocation across concurrent cache instances and supports
  legitimate post-rotation history re-grants. Producer full Swift 340/340,
  acceptance 195/195 and automated 16/16 passed. Claude Fable 5 max run
  `RUN-260720-cf2797` independently ACCEPTED with no Critical/High/Medium
  finding. Production crypto/runtime, real interop, signed package,
  cross-process ownership and physical leakage evidence remain deferred.)
- [x] `TASK-260712-3980vy` — macos-e2ee-live-ptt (accepted on exact rework
  commit `c9faa7ef4a5cc089ebfb83bdce11fadfcfe669b8`. The dormant sender reserves
  one witnessed generation and seals off capture callbacks; the receiver
  authenticates before jitter admission. Shared epoch plus `commitDigest`
  binds cross-device AAD while device-local revision remains setup/CAS-only;
  a two-installation fixture proves round-trip with deliberately skewed local
  revisions. Producer and independent runs passed strict format, focused
  10/10, Swift 350/350, acceptance 200/200 and automated 16/16. Claude Fable 5
  max run `RUN-260720-8f681f` ACCEPTED with no open Critical/High/Medium. The
  runtime/provider/capability stays dark and all physical, traffic, signed-app,
  memory, contention and platform-interop evidence remains manual/deferred.)
- [x] `TASK-260712-28zhpl` — windows-protected-media-send (accepted on exact
  rework commit `b2a4af69530545ede4b82f31a451c556ef7c536f`. The dormant Windows
  clip/track/saved-cue sender binds witnessed key state, target and recipients,
  validates authenticated provider output, persists ciphertext-only resumable
  drafts and converges cancel/expiry cleanup. Rework closes both first-review
  crash repros through private atomic prepare/publish, pre-reservation orphan
  rejection, bounded orphan recovery and idempotent missing-owned-plaintext
  cleanup. Producer and independent reviewer evidence passed focused 27/27
  plus race, key-state race, full Go plus race, vet, Windows amd64/arm64 blind
  compile, acceptance 205/205 and automated 16/16. Claude Fable 5 max run
  `RUN-260720-1e8fa2` ACCEPTED with zero open Critical/High/Medium. Stale
  ciphertext-only `.prepare-*` garbage collection is a Low follow-up; real
  provider/crypto, signed MSIX, native DPAPI/NTFS, traffic, memory and physical
  interop remain manual/deferred.)
- [x] `TASK-260712-1u57qz` — windows-protected-media-playback (accepted on
  exact producer commit `532774a1c37778a744acba53e897c6308435ebc0`.
  The production-dark Windows path authenticates the exact route, manifest,
  key-state lineage and every ciphertext record before decoder access; it
  revalidates current group or bounded history authority around every fetch
  and whole-object completion. Ciphertext-only durable cache ownership now
  includes the exact variant URL, shared actors preserve monotonic tombstones
  and a route-scoped revocation marker survives restart. Producer and reviewer
  evidence passed focused plus race, stream regressions plus race, full Go plus
  race, vet, Windows amd64/arm64 blind compile, acceptance 210/210 and automated
  16/16. Claude Fable 5 max run `RUN-260720-a152a9` added an adversarial
  aliasing probe, recomputed all 11 packet hashes and ACCEPTED with zero open
  Critical/High/Medium. Cross-process cache efficiency and all signed-MSIX,
  native provider/crypto/decoder, forensic, traffic, hardware and audible
  evidence remain manual/deferred.)
- [x] `TASK-260712-39vjzd` — windows-e2ee-live-ptt (accepted on exact producer
  commit `aee07339bcfe014b39edac10734f713d11333792`. The production-dark Windows
  bridge mirrors the accepted `BE` wire and shared commit-bound macOS AAD,
  reserves a witnessed cross-process `live_ptt` generation, seals only on the
  transport worker and authenticates before jitter/FEC/PLC. Retry, provider and
  caller ownership, sequence/duration, replay/nonce, authorization and
  exactly-once teardown are fail-closed; a two-installation fixture proves
  round trip under skewed local revisions. Producer and reviewer evidence
  passed focused 11 scenarios plus race, live regressions plus race, full Go
  plus race, vet, Windows amd64/arm64 blind compile, acceptance 215/215 and
  automated 16/16. Claude Fable 5 max terminal run `RUN-260720-21d7d3`
  ACCEPTED with zero open Critical/High/Medium after a fresh synchronous
  harness. Real traffic capture, signed MSIX, native provider/crypto/codec,
  physical audio/hardware and forensic evidence remain manual/deferred.)
- [x] `TASK-260712-2nppt6` — macos-encrypted-media-client-path (accepted on
  exact producer commit `3a64b1808ce990fbef2cfb737839a15cbd0f6cbb`.
  The production-dark macOS SwiftUI integration keeps protected media visibly
  blocked instead of silently downgrading, composes send/playback/live over
  one `MacE2EEKeyStateRepository` plus a retained abstract cross-process lease,
  and exposes fail-closed verification/revoke, current-epoch transfer,
  user-held recovery, bounded history grants and separate report-consent
  flows without retaining secrets or rendering stable identifiers. Claude
  Fable 5 max run `RUN-260720-c23a33` reproduced focused Swift 6/6, full Swift
  356/356, focused acceptance 5/5, automated 16/16 and release build, then
  ACCEPTED with zero open Critical/High/Medium. Two Low findings are
  non-blocking; all signed/notarized app, real Keychain/provider/codec,
  physical device/audio, traffic/memory/crash and moderation-storage evidence
  remains manual/deferred in `EPIC-260714-th54l3`.)
- [ ] `TASK-260712-2q4jbu` — windows-encrypted-media-client-path
- [ ] `TASK-260712-1bcpda` — e2ee-c4-c6-evidence-review-pack

## 17. P3 capture quality and diagnostics

Story: `STORY-260712-3pt00e` — P3 Capture quality and diagnostics.

- [x] `TASK-260712-1gmsvh` — freeze-capture-quality-contract (accepted on
  exact engineering commit `70d4cda548dc82025996b2587ac98bac6078ef49`,
  merged by PR #236 as `5163b7fbe21f12ac57dcf2de3a7e7a66c9359c13`.
  The dated candidate-neutral contract freezes one shared processor for
  recorded clip, five-second local self-test and live PTT, exact graph and
  callback ownership, synchronized memory-only render reference, honest
  route/effect/health/fallback states, distinct `-3 dBFS` input and `-1 dBFS`
  receiver ceilings, additive heartbeat/history decisions, privacy, rollback,
  and a 14-case objective plus blinded C3 rubric. Runtime still does not
  advertise `capture_quality_v1`. Dirty and clean exact-head acceptance passed
  16/16; hosted run `29561196208` passed 4/4. AEC/NS/AGC implementation,
  signed hardware, acoustic, accessibility and blinded evidence remain
  explicitly `not-run` in `EPIC-260714-th54l3`.)
- ↪ manual `TASK-260712-265o0f` — probe-windows-voice-processing-path
- ↪ manual `TASK-260712-2gaswa` — probe-macos-voice-processing-path
- [x] `TASK-260712-1pw1l1` — capture-diagnostics-capability-surface (accepted
  on exact engineering commit `c0a79b9239ea17326699c23245ca592220404df6`,
  merged by PR #238 at `273d460446ec16bb664256643fcdde2ecd600217`.
  The additive coordinator/Windows/Swift mirrors validate bounded route,
  lifecycle, quality, AEC/NS/AGC and input-health state, bind it to the exact
  advertised capability and authenticated socket generation, reject malformed
  and stale claims, and retain only defensive ephemeral snapshots. Both native
  seams expose the same mixed-version, route/effect/health and distinct
  `-3/-1 dBFS` presentation without granting microphone authority; diagnostics
  are categorical and content-free. Clean exact-head acceptance passed 16/16
  with `manualEvidence=not-run`; hosted run `29563206803` passed 4/4.
  Production builds still do not advertise `capture_quality_v1`; DSP, signed
  hardware, acoustic and accessibility evidence remains unexecuted in
  `EPIC-260714-th54l3`.)
- [x] `TASK-260712-39czd2` — capture-quality-regression-harness (accepted on
  exact engineering commit `5136950e8b8a8da9b0f9bd74e84db766315b4a76`,
  merged by PR #240 at `7d861c5f54375af5f7eb5114cb3d6fd4d835d6bc`.
  The stdlib-only harness generates 14 deterministic non-speech float32
  fixture classes with a checked-in content-addressed lock and evaluates one
  exact platform/build/workflow/route cell without trusting candidate audio
  metrics. It recalculates ERLE, residual, near-end preservation, noise,
  AGC/clipping and validates bounded runtime, lifecycle, 2% packet-loss and
  zero-post-cancel receipts. Negative self-tests reject bypass, near-end
  destruction, ceiling violation, realtime blocking, generation reuse,
  post-cancel emission, lock/hash tamper and path traversal. Clean exact-head
  acceptance passed 7/7 and hosted run `29565007783` passed 4/4. Synthetic
  correlation does not replace canonical STOI; signed app, physical hardware,
  acoustic, listening and physical resource evidence remains `not-run` in
  `EPIC-260714-th54l3`.)
- [x] `TASK-260712-2egweh` — macos-live-capture-effects (accepted on exact
  engineering commit `1ccbb16a30ee1d6c5c1d60e479e911d8ea24b4af`, merged
  by PR #242 at `a81e8fd3254ee342ba04594916e79298486e781b`. One shared
  macOS backend selects recorded clip, local self-test, or live PTT, enables
  public AVAudioInputNode voice processing, and applies the product-owned
  bounded AGC with +12 dB gain, 3 dB/s slew, and the final -3 dBFS input
  ceiling. Its realtime tap only downmixes into a fixed 16,384-sample mailbox
  through nonblocking signalling; resampling, DSP and client callbacks run on
  the serial worker. Headphone can become code-eligible for accepted only when
  native processing is active; speaker stays degraded/reference_unavailable,
  unknown and mismatched routes remain degraded, and live PTT fails closed as
  capture_quality_unsupported without fresh degraded consent. The distinct
  receiver -1 dBFS ceiling is unchanged, live/diagnostic audio is not
  persisted, and production still does not advertise capture_quality_v1.
  Clean exact-head acceptance passed both contract and Swift commands with 304
  Swift tests, `manualEvidence=not-run`; hosted run `29567251374` passed 4/4.
  Native deterministic AEC/NS C3, signed-app, physical speaker/headphone,
  Bluetooth/external-route, resource and blinded-listening evidence remains
  explicitly not-run in `EPIC-260714-th54l3`; the real-hardware checklist item
  is closed here only as routed, not as a claimed pass.)
- [x] `TASK-260712-wcdz08` — windows-live-capture-effects (accepted on exact
  engineering commit `fc127034ee05b6a850e9ac5ed4aff237c777bf37`, merged
  by PR #244 at `e15903b5f5d545885e0814b76e519449823d8409`. The
  shared WASAPI path requests the public Windows Communications category
  through a separately negotiated versioned extension, but deliberately does
  not treat category activation as AEC/NS proof: production keeps native
  effects unverified and route resolution unknown. Recorded clip, local
  self-test and live PTT share fresh quality generations, typed lifecycle and
  degraded states, plus the bounded product stage targeting -20 dBFS with +12
  dB maximum gain, 3 dB/s slew and the final -3 dBFS input ceiling; the
  receiver -1 dBFS ceiling is unchanged. Live PTT fails closed as
  capture_quality_unsupported without explicit degraded consent, and no live
  audio is persisted. Clean exact-head Windows acceptance passed 11/11 with
  `manualEvidence=not-run`; hosted run `29569443207` passed 4/4 including
  native C++ tests and signed MSIX packaging. Signed-app physical routes,
  render-reference alignment, AEC/NS C3, double-talk, resources, listening and
  accessibility remain explicitly not-run in `EPIC-260714-th54l3`; the
  real-hardware checklist item is closed here only as routed, not as a claimed
  pass.)
- [x] `TASK-260712-1getbv` — macos-capture-quality-ui (accepted on exact
  engineering commit `c589c59ce252f12d4c50e453a5bd1d260d13e6a9`, merged
  by PR #246 at `8706dbd88d11d02f59a2bf5a6878ca64524fc8e1`. The
  macOS shell now exposes local auto, speaker and headphone selection, explicit
  one-generation degraded consent, truthful route/lifecycle/AEC/NS/AGC/health
  state, and the distinct fixed -3 dBFS input and -1 dBFS receiver ceilings.
  A persistent window bar, foreground Escape, Command-period and menu item all
  invoke the local capture composition only; the coordinator has no microphone
  start, route or consent seam. English/Russian copy and non-color symbols
  distinguish accepted, degraded, unsupported, not-required, unavailable and
  faulted states plus the frozen failure reasons. Production clips and local
  self-test are wired; generic live_ptt presentation is covered, but the
  production MacLivePTTNode remains intentionally dark and no live UI pass is
  claimed. Clean exact-head acceptance passed 142 contract tests and 307 Swift
  tests, the release build passed, and hosted run `29571493442` passed 4/4.
  Real signed-app VoiceOver/focus, visual, TCC, hardware-route, acoustic and
  stop-latency checks remain `manualEvidence=not-run` in
  `EPIC-260714-th54l3`; the accessibility checklist item is closed only for
  source/unit semantics and explicit routing, not as a manual pass.)
- [x] `TASK-260712-39zh8g` — windows-capture-quality-ui (accepted on exact
  engineering commit `de40dcb71387e9e3e422e72adf6f999cb0572212`, merged
  by PR #248 at `def2aa2845175efac4a06f942cf2468e5b8e6ca7`. The native
  Win32 main window and notification-area menu now expose local auto, speaker
  and headphone selection, one-generation degraded consent, exact validated
  route/lifecycle/AEC/NS/AGC/health state, and the distinct fixed -3 dBFS input
  and -1 dBFS receiver ceilings. Active local capture replaces Record with a
  persistent native Stop; foreground Escape, Ctrl-period and the tray item use
  the same local workflow cancellation seam. English/Russian typed guidance
  covers permission, device, reference, route, input-health, processing and
  mixed-version failures, while a failed generation remains visible until the
  next attempt. Production clips and self-test are wired, but native effects
  remain honestly unverified, capture_quality_v1 is unadvertised, and shipping
  main still does not construct WindowsLivePTTNode. Clean exact-head Windows
  acceptance passed 11/11 with start/end dirty false and
  `manualEvidence=not-run`; hosted run `29573283583` passed 4/4 including the
  signed-MSIX packaging contract. Real signed-app Narrator/focus, DPI visual,
  permission, physical route/reconnect, acoustic and stop-latency checks remain
  in `EPIC-260714-th54l3`; checklist item 5 closes source/unit native-control,
  keyboard, DPI-aware layout, lifecycle and no-remote-start semantics only.)
- [x] `TASK-260712-1023d7` — capture-quality-integrated-regressions (accepted
  on exact engineering evidence head
  `93489c1cde6799e9b563fd4530712783aa29aa06`, merged by PR #250 at
  `97ec5ac77285e5c2a6c9f2cc19c375b79480e004`. Repository-built Windows and
  macOS safety adapters processed all 14 frozen fixtures across recorded clip,
  local self-test and live PTT on speaker, headphone and unknown routes: 18
  independent cells and 252 fixture runs with no cross-cell averaging. Every
  run enforced finite output, bounded +12 dB gain, 3 dB/s slew and the final
  -3 dBFS capture ceiling; separate assertions preserve the receiver -1 dBFS
  ceiling. Fresh generations, explicit degraded consent, fail-closed states,
  callback guards and existing hostile permission/device/route/cancel/lock/
  sleep/reconnect/rollback suites passed. Sanitized evidence retains only
  hashes, content-free metrics and exact blockers; no audio or device identity
  is retained. Clean exact-head acceptance passed all 16 commands with a clean
  start/end worktree; hosted run `29575552543` passed 4/4. Native Windows
  AEC/NS, signed macOS VPIO, render-reference age, acoustic ERLE/SNR, canonical
  STOI, blinded listening, accessibility and physical CPU/memory remain
  explicitly `manualEvidence=not-run` in `EPIC-260714-th54l3`; production
  capability advertising and C3 acceptance remain false.)
- ↪ manual `TASK-260712-2e80pr` — c3-evidence-capability-matrix

## 18. P3 security and engineering completion

Story: `STORY-260712-2ft5wd` — P3 Security acceptance and rollout. No
capability is released merely because another capability passed its gate.

- [x] `TASK-260712-3da0vz` — phase3-gate-matrix-evidence-contract (accepted
  on exact engineering commit `9d429325f1b342f0559b6cc4d604023de19af6e4`,
  merged by PR #252 at `45a83e7273358dc60903672ff946680492990f4e`.
  The frozen machine contract maps C1-C7 plus twelve section 21.4/exit gates to
  repository preflight commands, exact manual/review paths, artifact layouts,
  owners and truthful non-pass statuses. It enumerates all 16 ordered
  live_ptt/e2ee_media/soundboard_cues/automation flag postures, preserves four
  independent promotion decisions, binds one reviewed build/fixture/topology
  identity, and freezes the seven-day reset rubric. Approved Relux Works/Ivan
  Oparin legal, operations, mailbox, policy URL, moderation, hosting, Store and
  counsel defaults are consumed from the canonical approved input contract.
  Six remaining hardware/network/participant, deferred E2EE, independent
  reviewer, public external-record and observability inputs remain explicit
  blockers. The fail-closed campaign validator rejects source drift, false
  passes, missing/misclassified flag cells, averaging, invented reviewers or
  environments, raw evidence export, root/flag mismatch and artifact hash
  tampering. Clean exact-head acceptance passed all 16 commands with a clean
  start/end worktree and `manualEvidence=not-run`; hosted run `29576896538`
  passed 4/4. No C1-C7, independent-review, physical-hardware, beta, Store or
  promotion result is claimed.)
- [x] `TASK-260712-2uo81g` — phase3-observability-health-evidence-views
  (accepted on exact engineering commit
  `df2a410081d3be8384c84108179614927b7b22ef`, merged by PR #254 at
  `5e51965249134237c076c4e9fcf162c8e8179cde`. The authenticated, query-free,
  no-store operator snapshot exports only fixed-cardinality Live PTT lifecycle,
  drop and zero-retention gauges; capture route, lifecycle, AEC/NS/AGC and input
  health aggregates; canonical automation feature, execution, denial, revoke
  and audit aggregates; exact feature posture; build version; and a one-way
  environment reference. Public health exposes coarse per-subsystem runtime
  readiness: disabled optional capabilities remain healthy, while enabled
  missing, contradictory or prohibited-retention telemetry fails closed.
  Mouth-to-ear latency, jitter, capture callback, manual hardware, independent
  review, rollout/recovery and beta incident evidence remain visibly
  `client_evidence_required` or `not_run`; E2EE is honestly
  `deferred_unavailable`, and runtime health can never claim promotion. Hostile
  identifier/secret/path tests, targeted race, full Go test/vet and 159 Python
  contract tests passed. Clean exact-head acceptance passed all 16 commands
  with clean start/end and `manualEvidence=not-run`; hosted run `29578990210`
  passed 4/4. The external evidence stays in `EPIC-260714-th54l3`.)
- [x] `TASK-260712-3g0axs` — phase3-root-line-review (accepted on exact
  reviewed non-E2EE source candidate
  `d94f51644a3acf37601b4a869b4247380372f9ec`, tree
  `4e4cca878db806650eda6f1e1642051b87a18b93`; review packet
  `7388459356ec3a6ed976cdc779fec939adfa8d7b`, merged by PR #256 at
  `0d6f85d43909737ff717464d8f427ea315f870b2`. The deterministic manifest
  inventories all 75 first-parent intervals, 420 paths and 1,700 aggregate
  hunk headers from the Phase 2 handoff, with zero unmapped paths, exact task
  scope/AC hashes and explicit exclusion of four deferred E2EE tasks. Direct
  review found and fixed two High issues: macOS `audible_started` now requires
  render-consumed PCM instead of route activation, and new live/capture logs
  no longer expose raw identifiers or arbitrary errors. Runtime reject payload
  validation and a race-safe log fixture were also fixed and re-reviewed. Full
  coordinator and Windows vet/race suites, 308 Swift tests, 116 acceptance
  tests and clean exact-packet all-suite 16/16 passed; hosted run
  `29582027620` passed 4/4. No unresolved critical/high non-E2EE engineering
  finding remains. Manual real-app/hardware evidence is still `not-run`, E2EE
  remains `deferred-unavailable`, and independent review, production,
  promotion and beta claims remain blocked.)
- ↪ deferred E2EE `TASK-260712-1ulshp` — phase3-external-security-review-closure
  (open in `EPIC-260716-3qsztl`, blocked by the independent design review and
  complete C4-C6 evidence pack; no completion or progress credit claimed)
- [x] `TASK-260712-3j4a06` — phase3-independent-realtime-review (engineering
  pre-review accepted on exact packet commit
  `68afff5295ad395985d04cb18efc2872544e439c`, merged by PR #258 at
  `2ad49cdd89e1345696183240a15ab87165a88480`. The source-linked packet pins
  25 capture, encoder, protocol, jitter, mixer, callback, DSP, lifecycle and
  gate sources to root-reviewed candidate
  `d94f51644a3acf37601b4a869b4247380372f9ec`. Coordinator LivePTT/Phase3
  race groups passed ten repetitions, Windows passed four packages under race
  and ten repetitions, Swift passed 75 relevant tests in 14 suites, and the
  transport/capture group passed 23 tests plus 252 synthetic fixture runs.
  The clean exact-packet coordinator harness passed 7/7 with clean start/end;
  hosted run `29583827330` passed 4/4. One deliberately over-broad whole-store
  race×10 attempt exceeded Go's ten-minute package timeout on an unrelated
  identity rollback test and is explicitly recorded as non-counted; the
  scoped realtime store group then passed. No new Critical/High code finding
  remains, but this inline session did not claim independent review, hardware
  identity, C1-C3, audible echo/double-talk or packaged route evidence.
  Checklist items for physical artifacts and independent retest remain open;
  manual closure is `TASK-260712-flaiie`, external approval is
  `TASK-260717-3dbi2v`, and `live_ptt` activation and Phase 3 promotion remain
  blocked.)
- [x] `TASK-260712-1x5jfo` — phase3-independent-automation-review (engineering
  pre-review accepted on exact packet commit
  `a1dae4856f4bafa0c7679fddc19e3661691a4812`, merged by PR #260 at
  `e41f17144412b4bfc54e8351657070b92ed8fa1f`. The source-linked packet pins
  23 authority, cue, schedule, principal, runtime, Telegram, client, mixer and
  gate sources to root-reviewed candidate
  `d94f51644a3acf37601b4a869b4247380372f9ec`. The frozen ten-test
  adversarial coordinator group passed race×10, coordinator/Telegram passed
  race×10, exact previous-head automation rollback passed ten repetitions,
  Windows passed four packages under race×10, and Swift passed 19 tests in
  three suites. Clean exact-packet coordinator acceptance passed 7/7 with
  clean start/end; hosted run `29585744116` passed 4/4. A deliberately broad
  expression selected 54 store tests and exceeded Go's ten-minute package
  timeout while an unrelated scheduler test was running; it is explicitly
  recorded as non-counted and the frozen subset was rerun successfully. No
  new Critical/High code finding remains. Authority, idempotency, DST/no-
  catch-up, bounds, DND/block/Air rechecks, kill switches, opaque callbacks,
  local ceiling and no-microphone composition are repository-proved. The
  inline session did not claim implementation independence or signed-app C7;
  external closure is `TASK-260717-1pyg62`, manual closure is
  `TASK-260712-1gyohk`, and automation activation and Phase 3 promotion remain
  blocked.)
- [x] `TASK-260712-7ng1vs` — phase3-independent-privacy-store-review
  (engineering pre-review accepted on exact packet commit
  `5784985d02feb0471cc7cb389c7d3141dfad12b7`, merged by PR #262 at
  `bb2adae52ad1bf83c1e813adf16888bc97c9727e`. The machine packet pins 32
  moderation, consent, policy, legal, listing, client and Phase 3 gate sources
  to root-reviewed candidate `d94f51644a3acf37601b4a869b4247380372f9ec`.
  Moderation store, service/HTTP and content-policy groups passed race×10;
  exact previous-head moderation rollback passed ten repetitions; Windows
  passed four packages under race×10; and macOS passed 25 tests in four suites.
  Clean exact-packet coordinator acceptance passed 7/7 and hosted run
  `29587384257` passed 4/4. No new Critical/High technical or copy finding
  remains. Phase 1 copy truthfully discloses coordinator-readable non-E2EE
  audio, metadata, deletion/retention, recovery and Telegram boundaries;
  evidence access remains capability-gated, audited and TTL-bound. Approved
  defaults are canonical and policy source state is `proceed`, while Store
  submission remains `hold`, twelve screenshots remain manual, and exact WACK,
  IARC portal, build-certification, mailbox and publication evidence is not
  claimed. External closure is `TASK-260717-35bll1`; `e2ee_media`,
  `soundboard_cues`, `automation` and Phase 3 promotion remain blocked.)
- ↪ manual `TASK-260712-flaiie` — phase3-c1-c3-live-platform-matrix
- ↪ manual `TASK-260712-yj668d` — phase3-c4-c6-reviewed-e2ee-acceptance
- ↪ manual `TASK-260712-1gyohk` — phase3-c7-automation-safety-acceptance
- ↪ manual `TASK-260712-30xwu2` — phase3-rollout-rollback-recovery-drills
- [x] `TASK-260712-6mz9xg` — phase3-independent-migration-recovery-review
  (engineering pre-review accepted on exact packet commit `e68b59e`, merged by
  PR #264 at `8f5c15ae6f8867762ef4eeef17756e645be790c4`. The machine packet pins
  29 schema, rollback, recovery, feature-kill, client, backup and Phase 3 gate
  sources to root-reviewed candidate
  `d94f51644a3acf37601b4a869b4247380372f9ec`. The migration/recovery store
  matrix passed race×10 in 419.239 seconds, command/feature-kill passed race×10
  in 93.612 seconds, and the twelve-test exact-predecessor matrix passed ten
  repetitions in 483.746 seconds. Windows passed four packages under race×10,
  macOS passed 49 tests in five suites and 35 Phase 3/E2EE contract tests passed
  with production E2EE disabled. Clean exact-packet coordinator acceptance
  passed 7/7; hosted run `29589199967` passed 4/4. No new Critical/High
  technical finding remains. Actual E2EE fork/transfer/key-loss, destructive
  provider restore, signed mixed-fleet feature kills and manual drills are not
  claimed; checklist item 3 remains open. External closure is
  `TASK-260717-1sgb5r`, manual closure is `TASK-260712-30xwu2`, deferred E2EE
  remains in `EPIC-260716-3qsztl`, and all Phase 3 flags, beta and promotion
  remain blocked.)
- ↪ manual `TASK-260712-1actom` — phase3-beta-soak-incident-review
- [x] `TASK-260712-3b7bp4` — phase3-engineering-handoff-disclosures (accepted
  exact packet commit `3d3ef4a3d7f8419512b80efd4a09c0909155e230`,
  merged by PR #266 at `ebbf02d421fe29ec44cd89b51373357444b3e5bb`.
  The fail-closed handoff pins 35 source authorities, indexes all 19 C1-C7/NF
  gates, all 19 deferred manual tasks, all 18 deferred E2EE tasks, four
  implementation-independent Phase 3 approval tasks, rollback commands and
  conditional EN/RU disclosure surfaces. `live_ptt` is explicitly
  coordinator-readable and not E2EE; `e2ee_media` is absent and disabled; the
  Store draft has `eligibleForSubmission=false`. Exact-head clean all-suite
  acceptance passed 16/16 with 188 contract tests, coordinator vet/tests and
  previous-head rollback, protected-container test/race/cross-build, Windows
  vet/test/race/cross-build and 308 Swift tests in 52 suites; manual evidence
  remained `not-run`. Hosted run `29591121063` passed 4/4. No production
  artifact, physical C1-C7 result, independent approval, public policy,
  Partner Center action, signed rollout drill, beta day, E2EE implementation
  or release was claimed. The packet authorizes only the next root engineering
  completion audit.)
- [x] `TASK-260712-2b5685` — phase3-root-engineering-completion-audit
  (accepted exact audit commit
  `b6d0aeece5530f1a862949cc796fa407688fe381`, merged by PR #268 at
  `f108b62b2eea5314a9954daaa7f5b368c558768e`. The audit regenerated all
  eleven post-root first-parent merges and the 59-path 15,593/69-line delta,
  proving zero product/runtime, dependency-lock, workflow or deploy changes
  after frozen non-E2EE source
  `d94f51644a3acf37601b4a869b4247380372f9ec`. Focused coordinator Live PTT,
  Phase 3 observability and automation race spot checks passed. Exact-head
  clean all-suite acceptance passed 16/16 with 194 contract tests,
  coordinator/rollback/container/Windows stages and 308 Swift tests in 52
  suites; `manualEvidence=not-run` and the checkout was clean at both ends.
  Hosted run `29592847557` passed 4/4. Live PTT, capture quality, soundboard
  and automation repository engineering are complete with separate `hold`
  promotion decisions; E2EE remains deferred unavailable. No reviewed-scope
  Critical/High finding or unowned placeholder remains. The original epic,
  production, Store, manual C1-C7, independent approvals, rollout/recovery and
  beta remain open; accepted beta days remain zero.)

## Milestone gates

- P1 engineering closes after `TASK-260712-1xik11` freezes the reviewed build,
  automated evidence and exact manual-test handoff. It does not submit or claim
  acceptance in Partner Center.
- P2 engineering starts after P1 engineering closes; P3 engineering starts
  after `TASK-260712-3a0cf9` publishes the reviewed engineering packet. The
  seven-day P2 beta remains pending in the manual epic.
- `TASK-260712-2b5685` closes the current non-E2EE Phase 3 engineering
  sequence only. The original epic remains open for deferred E2EE, external
  approvals and every applicable task in `EPIC-260714-th54l3`.
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
