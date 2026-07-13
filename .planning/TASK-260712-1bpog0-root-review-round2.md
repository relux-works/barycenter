# TASK-260712-1bpog0 root review round 2

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **REWORK — the R1-R8 handoff is not accepted**

The producer corrected the concrete R1-R7 implementation defects and added a
real pinned-previous-HEAD harness. Root inspected the resulting production and
test code rather than accepting the reported green suite. One mandatory R8
composition proof is still absent, so independent review and downstream tasks
must not start yet.

## R8.1 — rollback evidence is split across non-equivalent tests (high)

The root R1 contract requires **two complete**
`new-on -> projection -> previous-binary mutation -> re-enable` cycles,
including quota change and the full legacy authority surface.

Current evidence is split:

- `TestR8ExactPreviousHEADAuthorityRoundTrip` executes pinned revision
  `e8bd240664a40b9cc78b974f3c34ad30712e2aa5` once, but does not run
  `ProjectIdentityForLegacyRollback` in that cycle.
- `TestR8TwoRollbackProjectionGenerationsPreserveQuotaChanges` performs two
  projection generations, but its rollback interval uses the current
  feature-off `Store`, not the pinned previous implementation, and exercises
  only a small mutation subset.
- `TestR8FullPreviousAuthorityMutationEmulationReconciles` is SQL emulation,
  not previous-code execution.

Those tests establish useful pieces, but not their composition. In particular,
they do not prove that each projected database can be opened and enforced by
the exact previous implementation, mutated through that implementation, then
reconciled and projected a second time with the new quota generation.

### Required correction

Add a deterministic tagged integration that performs **two full generations**.
For each generation it must:

1. create/prepare state through feature-on current code;
2. call the real rollback projection;
3. execute the exact pinned previous-HEAD `Store` API against that same DB;
4. during the previous-code interval, prove a projected disabled orbit rejects
   `LookupToken`, `PairSlot`, `AddMember`, and `ConsumeInvite`;
5. in one or more active fixture orbits in that same interval, exercise the
   required old surface: add member, rename, transfer primary, leave, pair,
   revoke, same-coordinate rebind, new slot, create orbit, and dissolve/delete;
6. return all old-minted plaintext tokens needed to verify authority after
   current-code reopen;
7. reopen feature-on and verify roles, ownership, revoked/left actors, node-only
   capability, old/new token validity, one-way projected-slot behavior,
   projection journal state, `foreign_key_check`, and `integrity_check`;
8. change quotas before generation two and prove its journal captures/restores
   those new values rather than generation-one values.

Mutations that cannot coexist on one orbit may use multiple fixture orbits, but
all must run within each exact-old interval. Current `Open(path)` and raw SQL may
prepare/assert fixtures; they may not substitute for the old mutation/enforcement
surface.

The integration must remain deterministic in CI: pinned full hash, checkout
history available, bounded subprocess contexts, no skip counted as evidence.

## R8.2 — evidence handoff must be new and claim-exact

The completed run reused the prior outcome resource, so task-board correctly
kept the task in development. The next run must attach a new task-scoped outcome
artifact (for example `TASK-260712-1bpog0_rework-r2-results.md`) with:

- exact changed-file hashes;
- explicit mapping of both complete cycles;
- unabridged tagged-test, full test, race, vet, build, formatting, diff, and
  board-validation results;
- honest remaining external boundaries.

Do not mark the task accepted. After this correction it still requires a fresh
independent reviewer and a new root line-by-line/hash audit.
