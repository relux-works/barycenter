## Status
done

## Review
required

## Task Class
code

## Estimate
estimated(fibonacci(13))

## Blocked By
- (none)

## Blocks
- TASK-260727-1f2cyl
- TASK-260727-1msjz6

## Checklist
- [x] Capture the failing macOS UI and exact surfaced error
- [x] Trace identity bootstrap and real Air creation through client coordinator and persistence
- [x] Identify whether create is committed before the stale-revision response
- [x] Specify the target Barycenter Air invite recovery and active-playback UX
- [x] Persist diagnosis and proposed architecture as an outcome resource
- [x] Gate Air UI on typed coordinator capability on macOS and Windows
- [x] Return a dedicated air_rooms_not_enabled error instead of revision_conflict
- [x] Rename identity onboarding to Barycenter and device terminology and hide setup actions after pairing
- [x] Make first-device recovery non-blocking and resumable through authenticated rotation
- [x] Create an Air and its initial member invite as one idempotent workflow
- [x] Separate mutation completion from refresh and preserve retry keys
- [x] Add Swift and Go regression tests and run relevant builds
- [x] Update README and architecture documentation
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [ ] If review does not accept the work — verdict evidence added and status routed by the explicit verdict branches

## Notes
2026-07-27 diagnosis: production health reports air_rooms_enabled=false and air_authority_state=airs_shadow. The coordinator authority gate maps this rollout state to revision_conflict, so the reported create did not commit. Proposed architecture is attached; implementation awaits product approval of non-blocking resumable recovery.
2026-07-27 implementation complete: Air capability gating, dedicated disabled error, Barycenter/device onboarding, resumable recovery, create-plus-invite workflow, stable retries, tests, and docs. Verification: Swift 360/360; Windows all packages plus Windows cross-compile; coordinator Air targets pass. Full coordinator rerun still has unrelated existing moderation-handler failures.
2026-07-27 Windows airfix binary installed on ssh host win. Local/uploaded/installed SHA-256 matched; previous executable and desktop shortcut retained as backups. Desktop Pulsar shortcut now targets the standalone airfix binary. Console session was disconnected, so GUI is ready for the next interactive launch but not left running.
2026-07-27 signed macOS airfix installed as /Applications/Pulsar.app build 958.1. Strict code-signature verification and installed hash passed; old app retained as /Applications/Pulsar.app.backup-20260727-airfix. Updated NodeApp and bundled go-librespot remain live; coordinator welcome, Spotify auth, clock, and state logs are healthy.
spawn agent resolution: Agent selection: codex via runtime_affinity
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=codex; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [reviewer] reviewer (codex) (run=RUN-260727-8022f4, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260727-8022f4)
agent completed: [reviewer] reviewer (codex) (exit=1)
spawn run completed: codex (run=RUN-260727-8022f4, pid=8701, exit=1)
spawn agent resolution: Agent selection: claude via explicit_override
spawn launch composition: degraded_contract_unavailable; contract=agents-infra.child-launch-composition; provider=claude; schema=1; diagnostic=composition_contract_unavailable; bare child launch retained
spawn queued: [reviewer] reviewer (claude) (run=RUN-260727-f35940, max_parallel=20)
spawn run started: [reviewer] reviewer (claude) (run=RUN-260727-f35940)
2026-07-27 reviewer verdict (RUN-260727-f35940, claude): ACCEPTED. Root cause verified from client/server/health evidence: requireAirsAuthoritativeTx returned ErrAirRevision for airs_shadow -> revision_conflict; now returns dedicated ErrAirRoomsDisabled -> 503 air_rooms_not_enabled. IA/terminology, capability gating (mac+win), create+initial-invite idempotent workflow with preserved retry keys, mutation/refresh separation, and resumable non-blocking recovery all implemented and symmetric. Tests verified independently: swift 360/360; pulsar-win all packages + GOOS=windows cross-compile; coordinator Air store+HTTP tests deterministic pass incl new 503/shadow cases; git diff --check clean. The 4 failing coordinator cmd tests (History/Moderation/Overlay) are pre-existing FLAKY failures — reproduced on clean-HEAD worktree with varying subset per run, none Air-related, diff is additive-only. Production stays airs_shadow; authority cutover deferred to TASK-260727-1f2cyl (rollback-gated) and E2E to TASK-260727-1msjz6. Recovery-before-activation relaxation is a disclosed, board-approved, test-covered product decision, not stop-the-line. Verdict artifact: BUG-260727-1hjfxi_reviewer-verdict.md
agent completed: [reviewer] reviewer (claude) (exit=0)
spawn run completed: claude (run=RUN-260727-f35940, pid=9319, exit=0)

## Precondition Resources
- [BUG-260727-1hjfxi_air-create-error.png](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_air-create-error.png)
- [BUG-260727-1hjfxi_mislabeled-bootstrap.png](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_mislabeled-bootstrap.png)

## Outcome Resources
- [BUG-260727-1hjfxi_air-onboarding-architecture.md](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_air-onboarding-architecture.md)
- [BUG-260727-1hjfxi_air-onboarding-verification.md](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_air-onboarding-verification.md) — Implemented architecture and final automated verification
- [BUG-260727-1hjfxi_windows-airfix-install.md](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_windows-airfix-install.md) — SSH installation receipt for the Windows Air onboarding build
- [BUG-260727-1hjfxi_macos-airfix-install.md](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_macos-airfix-install.md) — Local signed macOS installation receipt for the Air onboarding build
- [BUG-260727-1hjfxi_spawn-log_-reviewer--reviewer--codex-_RUN-260727-8022f4.log](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_spawn-log_-reviewer--reviewer--codex-_RUN-260727-8022f4.log) — System spawn log captured by task-board
- [BUG-260727-1hjfxi_spawn-log_-reviewer--reviewer--claude-_RUN-260727-f35940.log](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_spawn-log_-reviewer--reviewer--claude-_RUN-260727-f35940.log) — System spawn log captured by task-board
- [BUG-260727-1hjfxi_reviewer-verdict.md](file://BUG-260727-1hjfxi/BUG-260727-1hjfxi_reviewer-verdict.md) — Reviewer verdict: ACCEPTED — AC/DoD evidence, independent test verification, pre-existing flaky coordinator tests confirmed

## Created
2026-07-27T18:01:36Z

## Last Update
2026-07-27T19:14:34Z

## Assigned To
[reviewer] reviewer (claude)
