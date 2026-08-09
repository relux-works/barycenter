## Status
done

## Review
required

## Task Class
code

## Estimate
estimated(fibonacci(5))

## Blocked By
- BUG-260727-1hjfxi

## Blocks
- TASK-260727-1msjz6

## Checklist
- [x] Record the deployed coordinator version and pre-rollout Air capability state
- [x] Locate the authoritative feature flags, migration gate and deployment mechanism
- [x] Prove backup and rollback commands before changing production state
- [x] Deploy the minimal accepted Air authority configuration or build
- [x] Verify health, logs and a targeted create-plus-invite probe
- [x] Attach a redacted rollout receipt with explicit keep-or-rollback verdict
- [x] Code written per task description and AC
- [x] Relevant tests written for new or changed behavior and passing
- [x] Lint clean
- [x] Relevant build/validation commands run after changes and build not broken
- [x] New outcome artifact attached on the board with a task-scoped name when the work produces notes, logs, screenshots, or other deliverables
- [x] Important findings, decisions, anomalies, or regressions recorded in logbook when relevant
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [ ] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
spawn agent resolution: Agent selection: claude via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=claude; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [implementer] developer (claude) (run=RUN-260727-40363e, max_parallel=20)
spawn run started: [implementer] developer (claude) (run=RUN-260727-40363e)
Approach (developer): Production Air authority lives in the DB air_authority singleton state machine (links_authoritative -> airs_shadow -> airs_authoritative, plus rollback_hold), driven by store.CutoverLinksToAirs / RollbackAirsToLinks (generation-CAS, WAL checkpoints, divergence + rollback-unsafe gates). Prod is currently airs_shadow (health: air_rooms_enabled=false). GAP: no operator-invocable surface wires those store functions to the deployed binary. Deliverable: add one-shot operator CLI commands --air-cutover / --air-rollback mirroring --project-identity-rollback (stop service = single writer, open store, transition, print redacted before/after + air_rooms_enabled, exit). Rollback surfaces rollback_hold/ErrAirRollbackUnsafe as a non-zero exit telling the operator to restore from backup. Tests: store-level transition + config-wrapper. SCOPE HONESTY: real production is a remote Coolify deployment I cannot and must not mutate autonomously (outward-facing, hard-to-reverse). I prove the reversible rollout end-to-end against a production-like store copy under .temp/ (backup proven, cutover, health airs_authoritative, create+invite probe, clean rollback back to links, original untouched) and attach a redacted REHEARSED rollout receipt with the exact human trigger + keep/rollback verdict.
HANDOFF (developer -> review). Delivered the reversible Air-authority rollout MECHANISM the deployed binary was missing: one-shot operator commands --air-cutover / --air-rollback in coordinator/cmd/duet-coordinator/main.go (runAirAuthorityCommand + applyAirAuthorityTransition), mirroring the single-writer --project-identity-rollback pattern. Prints redacted before/after receipt; rollback of a diverged store HOLDS (rollback_hold, non-zero exit -> restore from backup). Tests: air_authority_command_test.go (4 tests, all green) cover shadow->authoritative + health flip, create+invite probe under authoritative, shadow-mode create rejection, clean rollback, rollback-hold safety, and guard rejections. Gates: gofmt/vet/go build ./... clean; go test store exit 0; the 2 failing cmd Moderation tests are PRE-EXISTING (verified identical failure with my changes git-stashed away) and match the BUG-260727-1hjfxi reviewer note. Rehearsed end-to-end against a production-like store copy with the SHIPPED binary + a live /healthz probe: backup proven (identical sha256), cutover -> air_rooms_enabled=true/authoritative/status ok, create+invite probe (parked air + open invite, no revision_conflict), clean rollback -> links, held rollback on diverged store (exit 1), backup-restore -> shadow; orbits_readable=2 at every step (existing state readable), no integrity errors. Artifacts on board: TASK-260727-1f2cyl_rollout-receipt.md + TASK-260727-1f2cyl_rehearsal.log. SCOPE BOUNDARY (item 4 left unchecked): real production is a remote Coolify deployment (/var/lib/duet volume) not reachable here, and flipping real production authority is a human-authorization-gated, outward-facing action. The receipt documents the exact production runbook + rollback trigger + KEEP-READY/GO verdict for a human/ops operator to execute; this developer task does not itself mutate real production.
agent completed: [implementer] developer (claude) (exit=0)
spawn run completed: claude (run=RUN-260727-40363e, pid=12868, exit=0)
spawn agent resolution: Agent selection: claude via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=claude; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [reviewer] reviewer (claude) (run=RUN-260727-e5ef6d, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260727-e5ef6d)
REVIEW VERDICT: BLOCKED — human-only production-execution gate + concrete external blocker (see TASK-260727-1f2cyl_review-verdict.md).

WORK IS ACCEPT-READY, not dev rework. Independently verified: go build ./... exit 0; gofmt/vet clean; 4 new air-authority tests green (cutover+health flip, create+invite probe, rollback-hold safety, guards, config wrapper); store tests PASS. The 2 failing cmd Moderation tests are PRE-EXISTING — reproduced identically on a clean git worktree at HEAD (no working-tree changes), and live in moderation_http_test.go untouched by this task. Rehearsal log is real (shipped binary, live /healthz air_rooms_enabled:true+airs_authoritative, matching-sha256 backup, held rollback exit 1, backup-restore to airs_shadow, orbits_readable=2 throughout). Architecture fit good: mirrors single-writer --project-identity-rollback pattern; safety in the store state machine from BUG-260727-1hjfxi; no secrets printed.

WHY BLOCKED not DONE: terminal AC (deployed health endpoint reports Air rooms enabled) is real-production state. Real prod is a remote Coolify deploy unreachable from this env; flipping live prod authority is outward-facing/hard-to-reverse and human-authorization-gated. Prod is still airs_shadow. Marking DONE would falsely assert prod is live and mislead downstream TASK-260727-1msjz6 (manual human acceptance journey). Not to-dev: code is complete/correct.

HUMAN DECISION NEEDED (pick one): (A) authorize+run the documented runbook against real prod and attach redacted /healthz receipt, then accept; or (B) re-scope this task to the story-level bar (parent STORY AC: rollback "rehearsed against production-shaped data" — already satisfied) and accept as DONE, tracking the actual prod flip under TASK-260727-1msjz6.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260727-e5ef6d, pid=21777, exit=0)
2026-07-27 HUMAN AUTHORIZATION: Ivan explicitly replied давай after being asked to authorize enabling Air on the production coordinator and running the Mac-to-Windows two-independent-Barycenter E2E. Proceed with the reviewed production runbook; retain rollback-first safety and redacted evidence.
spawn agent resolution: Agent selection: claude via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=claude; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [implementer] developer (claude) (run=RUN-260727-ce0b11, max_parallel=20)
spawn run started: [implementer] developer (claude) (run=RUN-260727-ce0b11)
HANDOFF (developer -> review). LIVE production Air authority ENABLED per Ivan authorization (option A). Production is Coolify app barycenter (id10) as a docker container in Colima VM on host relux; image acb1469; volume barycenter-data->/var/lib/duet; duet uid100. NOT systemd (runbook assumption corrected). Executed rollback-first: (1) read-only pre-state (airs_shadow, orbits9, status ok); (2) full cutover/rollback/probe cycle rehearsed against a READ-ONLY copy of the REAL prod DB (9 orbits) -> cutover ok, ephemeral /healthz authoritative, create+invite probe parked-air+open-invite no revision_conflict, clean rollback + held-rollback(exit1) + backup-restore, orbits_readable=9 throughout; (3) LIVE flip: docker stop -> in-volume backup duet.db.TASK-260727-1f2cyl-pre-air-cutover (sha256 identical 7aa31d60, db+wal+shm, duet-owned) -> --air-cutover as duet (airs_shadow->airs_authoritative gen2, result=ok exit0) -> docker start. VERIFIED: public https://barycenter.relux.works/healthz reports air_rooms_enabled:true + air_authority_state:airs_authoritative + status:ok + orbits:9; container logs: sessions restored, orbits warmed up count=9 active_airs=1, error/integrity/migration matches=0. Live kept at divergence=0 (probe proven on real-data copy, not injected into live) so the CLEAN --air-rollback stays available. Delivery mechanism: mechanism (--air-cutover/--air-rollback in main.go) was uncommitted+absent from deployed acb1469; cross-compiled linux/amd64 operator binary run via one-off container from the same coordinator image (as duet) against the live volume -> zero image rebuild, zero migration risk (store schema identical acb1469..HEAD; uncommitted air.go delta = ErrAirRoomsDisabled only, non-schema). GATES: gofmt/vet/go build ./... clean; air_authority tests 4/4 exit0; store tests exit0. ANOMALIES logged: (a) /healthz version string git-3565c1e is STALE (real code=acb1469, use image tag); (b) no litestream sidecar running -> explicit DB copy is the only backup. Rollback anchor kept in-volume + tooling at relux:~/aircutover-1f2cyl/; runbook in TASK-260727-1f2cyl_rollout-receipt-executed.md. Unblocks TASK-260727-1msjz6. VERDICT: KEEP. Ready for review.
agent completed: [implementer] developer (claude) (exit=0)
spawn run completed: claude (run=RUN-260727-ce0b11, pid=24618, exit=0)
spawn agent resolution: Agent selection: claude via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=claude; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [reviewer] reviewer (claude) (run=RUN-260727-9d505e, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260727-9d505e)
REVIEW VERDICT (cycle 2): ACCEPTED -> done. Human authorization on record (Ivan davai, option A); live prod flip executed and independently verified by reviewer. Terminal AC VERIFIED LIVE: public https://barycenter.relux.works/healthz reports phase2.air_rooms_enabled=true + air_authority_state=airs_authoritative + status=ok + orbits=9 (no longer airs_shadow) — checked twice, stable. All AC gates green: redacted before/after receipt (identity/version/Air state), safe create+invite probe on a read-only copy of the REAL prod DB (parked air + open invite, no revision_conflict), orbits_readable=9 throughout, logs integrity/migration matches=0, explicit rollback command+trigger documented with sha256-identical in-volume backup. Code: mirrors single-writer --project-identity-rollback pattern; gofmt/vet/build clean; 4 air-authority tests green 3/3 deterministically; store tests PASS. Full cmd package has FLAKY pre-existing History/Overlay/Moderation failures — PROVEN pre-existing (same flaky set fails on a clean git worktree at HEAD with zero working-tree changes); none touch air-authority. Non-blocking follow-ups noted in verdict artifact: (1) commit/merge the --air-cutover/--air-rollback mechanism + rebuild image so rollback tooling is reproducible from source, not host-staged binary; (2) stale /healthz version string git-3565c1e (real=acb1469); (3) no litestream sidecar, in-volume DB copy is sole backup. Unblocks TASK-260727-1msjz6.
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260727-9d505e, pid=34422, exit=0)

## Precondition Resources
(none)

## Outcome Resources
- [TASK-260727-1f2cyl_spawn-log_-implementer--developer--claude-_RUN-260727-40363e.log](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_spawn-log_-implementer--developer--claude-_RUN-260727-40363e.log) — System spawn log captured by task-board
- [TASK-260727-1f2cyl_rollout-receipt.md](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_rollout-receipt.md) — Redacted Air-authority rollout receipt: reversible enable/rollback commands, rehearsed backup+cutover+health+probe+rollback evidence, production runbook, keep-or-rollback verdict
- [TASK-260727-1f2cyl_rehearsal.log](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_rehearsal.log) — Full standalone-exit-code log of the rollout rehearsal against a production-like store copy
- [TASK-260727-1f2cyl_spawn-log_-reviewer--reviewer--claude-_RUN-260727-e5ef6d.log](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_spawn-log_-reviewer--reviewer--claude-_RUN-260727-e5ef6d.log) — System spawn log captured by task-board
- [TASK-260727-1f2cyl_review-verdict.md](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_review-verdict.md) — Reviewer verdict: BLOCKED on human-only production execution; mechanism+tests+rehearsal independently verified accept-ready; pre-existing moderation failures confirmed at clean HEAD; two options + exact human decision needed
- [TASK-260727-1f2cyl_spawn-log_-implementer--developer--claude-_RUN-260727-ce0b11.log](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_spawn-log_-implementer--developer--claude-_RUN-260727-ce0b11.log) — System spawn log captured by task-board
- [TASK-260727-1f2cyl_rollout-receipt-executed.md](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_rollout-receipt-executed.md) — EXECUTED Air-authority rollout receipt: real Coolify/Colima deploy facts, before/after live /healthz, backup sha256, live cutover result=ok, gate table, KEEP verdict, adapted rollback runbook
- [TASK-260727-1f2cyl_rehearsal-realdata.log](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_rehearsal-realdata.log) — Full cutover/rollback/probe rehearsal against a read-only copy of the REAL production DB (9 orbits); standalone exit codes; orbits_readable=9 throughout
- [TASK-260727-1f2cyl_live-flip.log](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_live-flip.log) — Live production cutover transcript: stop/backup(sha256-identical)/cutover(result=ok)/start, all exit 0
- [TASK-260727-1f2cyl_spawn-log_-reviewer--reviewer--claude-_RUN-260727-9d505e.log](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_spawn-log_-reviewer--reviewer--claude-_RUN-260727-9d505e.log) — System spawn log captured by task-board
- [TASK-260727-1f2cyl_review-verdict-accepted.md](file://TASK-260727-1f2cyl/TASK-260727-1f2cyl_review-verdict-accepted.md) — Cycle-2 reviewer verdict: ACCEPTED. Terminal AC verified live (public /healthz air_rooms_enabled:true + airs_authoritative); AC gate-by-gate table; air-authority tests 3/3 green; package flakiness proven pre-existing at clean HEAD; 3 non-blocking ops follow-ups.

## Created
2026-07-27T19:04:35Z

## Last Update
2026-07-27T20:16:38Z

## Assigned To
[reviewer] reviewer (claude)
