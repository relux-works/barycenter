# P2 Air lifecycle regression rehearsal

- Date: 2026-07-15
- Task: `TASK-260712-3nq0tq`
- Scope: repository automation and synthetic coordinator load only
- Manual application/hardware evidence: not run; owned by the separate manual-test epic

## Result

The Air story has a deterministic regression harness at
`scripts/acceptance/run_air_regression.py`. It runs the canonical store,
control-plane, runtime, approach-alias and secure Telegram callback paths and
emits a machine-readable artifact. The synthetic production loop reaches the
frozen initial capacity shape with one authoritative Air controller:

```json
{
  "barycenters": 8,
  "pulsars": 20,
  "load_commands": 20,
  "unique_targets": 20,
  "duplicate_commands": 0,
  "runtime_instances": 1,
  "legacy_groups": 0
}
```

This is a command/fanout rehearsal, not a throughput, decoder-memory or
audible-output benchmark. Those claims require the later streamed-track and
integrated acceptance gates.

## B2-B4 and lifecycle coverage

| Requirement | Automated evidence |
| --- | --- |
| One saved/active pointer per Barycenter; switch fencing | `TestAirSchemaFreshRepositoryLifecycleAndOneActiveInvariant`, `TestConcurrentAirLifecycleChangesHaveOneTransactionalWinner`, `TestAirRuntimeSwitchAdvancesBothOwnershipRevisions`, `TestAirRuntimeSwitchDetachesOldOwnerBeforeJoiningNewAir` |
| Invite replay, concurrent consume, role and capacity enforcement | `TestAuthorizedAirControlLifecycleIdempotencyAndRestart`, `TestAuthorizedAirConcurrentConsumeAndCapacity`, `TestAuthorizedAirGovernanceTransferLeaveAndDissolve` |
| Exact current-member union, no saved-Air transitivity, no duplicate commands | `TestAirRuntimeOwnsExactCurrentUnionAndWarmupIsLazy`, `TestAirRegressionEightBarycentersTwentyPulsarsExactFanout` |
| Join during an active track versus old voice/overlay | `TestAirRuntimeJoinCatchesCurrentTrackButNeverOldVoiceOverlay` |
| Leave during overlay prepare/playback | `TestAirOverlayPrepareAndPlaybackCancelOnlyLeavingBarycenter` |
| Clean leave, dissolve and lazy parking | `TestAirRuntimeLeaveStopsOnlyCallerThenParksBelowTwoMembers`, `TestAuthorizedAirGovernanceTransferLeaveAndDissolve` |
| Restart and stale async-result fencing | `TestAirRuntimeOwnsExactCurrentUnionAndWarmupIsLazy`, `TestAirRuntimeRejectsAsyncCompletionAfterOwnershipSwitch` |
| `/approach` and `/apart` compatibility without dual authority | `TestAirApproachAliasLifecycleIdempotencyRestartAndCallerLocalApart`, `TestMigratedApproachAliasRestartAndRollbackCannotResurrectStaleLink`, `TestApproachAliasesUseOnlyAirAuthorityAfterCutover` |
| Opaque, actor-bound Telegram callbacks | `TestTelegramAirCallbacksAreOpaqueActorBoundExpiringAndAtomic`, `TestTelegramAirLifecycleParityAndRedaction` |
| Windows and macOS frozen client/UI projections | full repository acceptance reruns Go Windows Air fixtures and Swift `AirAppClient`/shell fixtures; no real UI interaction is claimed |

The rehearsal found and closed one lifecycle seam in the generic transmission
scheduler: an accepted overlay target was rechecked for binding, DND and block
state but not for continued active membership in its accepted Air. Runtime
recheck now cancels only a leaver's preparing target, or moves only a leaver's
playing target to cancelling, with the stable `approach_left` reason. The
immutable accepted target snapshot is not expanded, so joiners still do not
receive an old overlay.

## Migration and rollback

The harness covers transactional backfill retry, production-shaped cutover
failure, conflicting legacy links, rollback hold, post-cutover legacy mutation
detection, and single-authority approach aliases. The repository-wide
acceptance suite additionally runs the build-tagged exact-predecessor test
`TestAirExactPreviousCoordinatorLegacyServicePreservesPhase2Rows`. Together
these prove additive Air rows, deterministic retry, preservation by the
previous coordinator and no simultaneous legacy/Air delivery authority.

## Reproduction

```sh
scripts/acceptance/run_air_regression.py \
  --output .temp/acceptance/task-260712-3nq0tq-air-regression.json
```

The artifact is deliberately labelled `repository-automated-only` and
`manualEvidence: not-run`.

## Explicit downstream gaps

- B1 streamed-track catch-up, range playback, decoder memory and sustained
  performance belong to `STORY-260712-2ori1t` and its regression/handoff tasks.
- B5 explicit-target ACL, inbox visibility and rights revocation belong to
  `STORY-260712-ob1tx2` and its regression/handoff tasks.
- Integrated mixed-fleet scale, rollout/rollback and the seven-day beta belong
  to `STORY-260712-1qfbiw`.
- Real Windows/macOS application interaction, real Telegram transport,
  audible playback and physical-device behavior remain in the manual-test
  epic. This engineering task does not convert those into passing evidence.
