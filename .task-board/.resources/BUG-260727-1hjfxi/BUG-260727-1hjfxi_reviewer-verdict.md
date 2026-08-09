# Reviewer verdict — BUG-260727-1hjfxi

Date: 2026-07-27
Reviewer run: RUN-260727-f35940 (claude)
Verdict: **ACCEPTED → done**

## Scope reviewed

Diagnosis + product architecture + implementation for the Air-creation
stale-revision failure and the confused Barycenter/Air onboarding.

## Acceptance criteria

1. **Root cause supported by client/server/log evidence — MET.**
   - Health evidence: production `phase2.air_rooms_enabled=false`,
     `air_authority_state=airs_shadow`.
   - Server: `store.requireAirsAuthoritativeTx` returned `ErrAirRevision` for
     any non-`airs_authoritative` mode, which `airStoreError` mapped to the
     `revision_conflict` HTTP code; the client rendered "The Air changed
     elsewhere." The create is rejected by the authority gate before commit, so
     nothing was persisted. Confirmed by reading the gate and its callers.
   - Fix: gate now returns the dedicated `store.ErrAirRoomsDisabled` →
     HTTP 503 `air_rooms_not_enabled` (`air.go`, `air_http.go`, `onboarding.go`,
     `telegram_air.go`). `revision_conflict` is reserved for real CAS mismatches.

2. **Every user-visible object has one name/one responsibility — MET.**
   Installation (device) / Barycenter (private home) / Air (shared room) model
   documented and applied. Copy renamed throughout (`Start Barycenter`,
   `Connect device`); "Air" reserved for multi-Barycenter rooms. README and
   `docs/idea-air-rooms.md` updated.

3. **Flows specified with error/retry semantics — MET.**
   Architecture doc specifies first-device setup, Air create, cross-device join,
   recovery, active-playback. Implemented: capability gate (`availability()`
   from `/healthz`), create+initial-invite as one idempotent workflow, preserved
   idempotency retry keys, and separation of mutation completion from projection
   refresh (`refreshAfterConflict` reloads via `loadProjection()` directly
   instead of the mutation-guarded `refresh()` that previously dropped the
   reload). Mirrored on macOS and Windows.

4. **Decomposable follow-ups; production cutover gated — MET.**
   Two follow-ups created and blocked by this item:
   `TASK-260727-1f2cyl` (controlled Air-authority rollout with proven rollback)
   and `TASK-260727-1msjz6` (two-independent-Barycenters real-hardware E2E).
   Production stays in `airs_shadow` (Air UI hidden) until the rollout task; the
   client changes are safe under the current shadow state.

## Definition of Done

- Implementation matches AC — yes.
- Solution fits architecture — yes. Preserves the security model (hash-only
  recovery, OS-protected credentials, idempotency keys), reuses the existing Air
  error taxonomy, and keeps macOS/Windows behavior symmetric.
- Tests green — verified independently:
  - `swift test` (node-app): **360/360 in 58 suites**.
  - `pulsar-win`: all 4 packages pass; `GOOS=windows` cross-compile succeeds.
  - Coordinator Air tests (`internal/store` Air + `cmd` Air) pass deterministically,
    including the new `air_rooms_not_enabled` 503 and shadow-mode create tests.
  - The 4 failing coordinator `cmd` tests (History/Moderation/Overlay) are
    **pre-existing flaky failures**: reproduced on a clean-HEAD worktree with a
    different failing subset per run; none are Air-related and the diff is purely
    additive to that package. Not a regression from this change.
  - `git diff --check` clean.

## Notes / non-blocking observations

- The change deliberately relaxes the prior invariant that a new Barycenter
  cannot activate before a recovery file is saved (now activates immediately;
  recovery is a resumable, rotatable safety action). This was an explicit,
  disclosed product decision, is an approved board checklist item, is covered by
  regression tests (activation-before-export; rotate-after-restart;
  retain-material-on-export-failure), and its production exposure remains gated
  behind the separate rollout follow-up. Not a stop-the-line.
- Pre-existing flaky coordinator `cmd` tests should be tracked separately; they
  predate and are unrelated to this work.
