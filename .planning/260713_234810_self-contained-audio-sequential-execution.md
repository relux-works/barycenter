# Sequential execution plan: Self-contained Pulsar Audio

- Date: 2026-07-13
- Engineering epic: `EPIC-260712-3agrc1` — Self-contained Pulsar Audio engineering
- Manual test epic: `EPIC-260714-th54l3` — Manual real-app hardware testing
- Baseline: `main` at merge commit `38ebd385e105eb2f6c7012c608cd1debfa3aad5e` (PR #9)
- Combined inventory: 205 original tasks; 51 accepted, 154 remain.
- Routed inventory: 186 engineering tasks (51 accepted, 135 remain) and 19
  deferred manual-test tasks (0 accepted, 19 remain).

## Execution status

- Started: 2026-07-14
- Mode: strict sequential inline execution; no task-board spawn workflow
- Current engineering task: `TASK-260712-30abcm` — macos-microphone-capture-engine
  (strict kickoff waits only for PR #51 merge)
- Next engineering task: `TASK-260712-9i5se7` — windows-main-window-tray-shell
- Most recently accepted: `TASK-260712-2lrpc0` — builtin-cue-temp-media-contract
- Current branch: `task/task-260712-2lrpc0-builtin-cue-temp-media-contract`
- Current external-input gate: all seven legal/operations groups are approved
  by Ivan Oparin; exact head `3b12371` passed all four hosted jobs in run
  `29338589269`; tracking head `5af1b56` passed all four jobs in run
  `29339017452`. PR #29 landed at merge
  `e588fc9b727d6264c289f69cc97ea77e4987f939`.
- Current external-action ledger: `EPIC-260714-zmnd4n`. DNS inspection found
  no MX for `barycenter.live`; provider-side routing and synthetic delivery for
  the approved mailboxes are tracked as `TASK-260714-200ib8` and do not block
  reversible best-effort engineering. Store submission remains fail-closed.
- Accepted overall: 51 / 205 tasks (approximately 24.9%); 154 remain
- Engineering progress: 51 / 186 tasks (approximately 27.4%); 135 remain
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
