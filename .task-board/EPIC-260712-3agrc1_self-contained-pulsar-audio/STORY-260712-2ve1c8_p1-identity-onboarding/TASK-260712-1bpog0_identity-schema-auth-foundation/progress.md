## Status
done

## Assigned To
[reviewer] reviewer (codex)

## Created
2026-07-12T15:30:16Z

## Last Update
2026-07-13T11:55:32Z

## Blocked By
- (none)

## Blocks
- TASK-260712-m5264f
- TASK-260712-2xkyot
- TASK-260712-38qsku
- TASK-260712-2hcq1g
- TASK-260712-2kec2s
- TASK-260712-3mcof4
- TASK-260712-3n36ny

## Checklist
- [x] Add additive migration and backfill coverage
- [x] Cover hash-only persistence and actor lookup paths
- [x] Preserve legacy pair token validity and slot ownership
- [x] Gate the new resolver behind self_service_onboarding
- [x] Code written per task description and AC
- [x] Relevant tests written for new or changed behavior and passing
- [x] Lint clean
- [x] Relevant build/validation commands run after changes and build not broken
- [x] New outcome artifact attached on the board with a task-scoped name when the work produces notes, logs, screenshots, or other deliverables
- [x] Important findings, decisions, anomalies, or regressions recorded in logbook when relevant
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [x] If problems found — notes added and status set to to-dev

## Notes
spawn queued: [implementer] developer (codex) (run=RUN-260713-4409e2, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-4409e2)
Implemented additive actor/auth schema, legacy backfill/reconciliation, hash-only credential foundation, capability-aware ActorContext resolution, feature gating, and fail-closed rollback projection. Full uncached Go suite, store race/coverage, vet, build, board validation, and diff check pass. Evidence: TASK-260712-1bpog0_results.md. Independent and root line-by-line review pending.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-4409e2, pid=29386, exit=0)
spawn queued: [reviewer] reviewer (codex) (run=RUN-260713-14420d, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260713-14420d)
agent completed: [reviewer] reviewer (codex) (exit=1)
spawn run completed: codex (run=RUN-260713-14420d, pid=44861, exit=1)
spawn queued: [reviewer] reviewer (codex) (run=RUN-260713-5f29d6, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260713-5f29d6)
Independent review verdict: return to development. HIGH: control and node hash domains can alias because provisioning accepts a node token as control and resolver checks control first; this violates node-only capability. HIGH: ProvisionInstallationSecrets can overwrite credentials for revoked, left, or already-provisioned targets without target lifecycle/generation validation. HIGH: unconstrained orbits.status repair runs foreign_key_check before rollback reconciliation, so an old-binary dissolved orbit with stale additive children aborts before cleanup. MEDIUM: mandatory full previous-binary rollback, ambiguity, combined migration, projection cycle/crash, and re-enable tests are missing. All existing full, targeted, race, vet, build, format, diff, and board checks passed. Evidence: TASK-260712-1bpog0_independent-review.md.
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-5f29d6, pid=50770, exit=0)
Root round-1 line-by-line verdict: REWORK. Independent findings confirmed; root adds AUTOINCREMENT high-water preservation, transactional LeaveOrbit/Bootstrap serialization, migration cleanup error propagation, and atomic DDL evidence. Mandatory contract: TASK-260712-1bpog0_root-review-round1.md. No code accepted.
spawn queued: [implementer] developer (codex) (run=RUN-260713-948dbe, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-948dbe)
R1-R8 rework implemented. Exact pinned previous coordinator e8bd240664a40b9cc78b974f3c34ad30712e2aa5 executed through its real Store API; old-minted rebound/new node tokens resolve node-only after current reconciliation. R1-R8 repeated x10, pinned gate x3, full uncached Go suite, store race, HTTP/config repeated tests, vet, tagged vet, build, gofmt, diff check, and board validation pass. Outcome updated: TASK-260712-1bpog0_results.md. Residual evidence boundaries: no real OS power-loss kill and no external GitHub Actions run; independent and root reviews remain required.
agent completed: [implementer] developer (codex) (exit=0)
spawn completion blocked: no new task-scoped outcome artifact was attached. Add an outcome resource named like TASK-260712-1bpog0_results.md and then set status back to to-review.
spawn run completed: codex (run=RUN-260713-948dbe, pid=36113, exit=0)
Root round-2 preliminary line-by-line verdict: REWORK. R1-R7 corrections are present, but R8 remains split across one exact-old non-projection run and two current-feature-off projection runs; the required two complete new-on -> projection -> exact previous HEAD -> re-enable generations are not demonstrated. Mandatory contract: TASK-260712-1bpog0_rework-guard-r2.md. No code accepted.
spawn queued: [implementer] developer (codex) (run=RUN-260713-4dbe4d, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-4dbe4d)
R2 rework: added deterministic two-generation feature-on -> projection -> exact pinned previous HEAD -> feature-on reconciliation coverage on one database. Each exact-old interval proves disabled LookupToken/PairSlot/AddMember/ConsumeInvite denial and exercises add/name/transfer/leave/pair/revoke/rebind/new-slot/create/delete/dissolve through the old Store API. Generation two captures/restores changed quotas 3/7 instead of 5/10. Tagged repeated/CI-shaped gates, full uncached suite, store race, vet, tagged vet, build, gofmt, diff, and board validation pass. New evidence: TASK-260712-1bpog0_rework-r2-results.md. Fresh independent review and root hash/line audit remain required.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-4dbe4d, pid=50631, exit=0)
spawn queued: [reviewer] reviewer (codex) (run=RUN-260713-e9faff, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260713-e9faff)
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-e9faff, pid=57241, exit=0)
ROOT ACCEPTANCE R3: accepted after complete prior production review, exact R2 driver/CI/outcome review, fresh independent approval, final hash audit, and root-reproduced exact-old/full/race/vet/build/R1-R8/HTTP/config gates. Concurrent Windows LOGBOOK append was isolated and identity entries remained intact. Evidence: TASK-260712-1bpog0-root-acceptance-r3.md. Downstream tasks may rely on this foundation.

## Precondition Resources
- [p1-identity-model.puml](file://TASK-260712-1bpog0/p1-identity-model.puml) — Identity domain model for actor, membership, credential, and invite design
- [TASK-260712-1bpog0_implementation-guard.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_implementation-guard.md) — Mandatory implementation and evidence guardrails
- [TASK-260712-1bpog0_review-guard.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_review-guard.md) — Independent identity migration and security review requirements
- [TASK-260712-1bpog0_rework-guard-r1.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_rework-guard-r1.md) — Mandatory root R1-R8 identity/security/migration corrections and regression tests
- [TASK-260712-1bpog0_rework-guard-r2.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_rework-guard-r2.md) — Mandatory exact-old two-generation rollback composition correction

## Outcome Resources
- [TASK-260712-1bpog0_spawn-log_-implementer--developer--codex-.log](file://TASK-260712-1bpog0/TASK-260712-1bpog0_spawn-log_-implementer--developer--codex-.log) — System spawn log captured by task-board
- [TASK-260712-1bpog0_results.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_results.md) — R1-R8 implementation, exact previous-HEAD evidence, test map, verification, residual risks, and worktree scope
- [TASK-260712-1bpog0_spawn-log_-reviewer--reviewer--codex-.log](file://TASK-260712-1bpog0/TASK-260712-1bpog0_spawn-log_-reviewer--reviewer--codex-.log) — System spawn log captured by task-board
- [TASK-260712-1bpog0_independent-review.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_independent-review.md) — Independent security, migration, rollback, and authorization review verdict
- [TASK-260712-1bpog0_root-review-round1.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_root-review-round1.md) — Root line-by-line identity review and mandatory R1-R8 rework contract
- [TASK-260712-1bpog0_root-review-round2.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_root-review-round2.md) — Root R2 review: split rollback evidence does not satisfy the two complete exact-old cycles
- [TASK-260712-1bpog0_rework-r2-results.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_rework-r2-results.md) — R8.1 exact-old two-generation rollback composition implementation and verification evidence
- [TASK-260712-1bpog0_independent-review-r3.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0_independent-review-r3.md) — Independent post-R2 identity migration, authorization, concurrency, and exact-old rollback review
- [TASK-260712-1bpog0-root-acceptance-r3.md](file://TASK-260712-1bpog0/TASK-260712-1bpog0-root-acceptance-r3.md) — Root final line-by-line, hash, and test acceptance after R2 and independent R3 review
