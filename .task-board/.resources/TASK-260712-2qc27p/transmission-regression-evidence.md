# P1 transmission regression evidence

- Task: `TASK-260712-2qc27p`
- Story: `STORY-260712-25lysg`
- Engineering base: `8d2b7d3825536ed9dc732f1e86040edc227a7acf`
- Exact engineering code: `c60bd99ed4717a62b69a10338e5b13b39001e419`
- Pull request: `#27`
- Hosted exact-code CI: `29333494719`
- Manual test program: `EPIC-260714-th54l3`

## Evidence boundary

This task proves deterministic repository, API, scheduler, codec and node-client
behavior. It does not claim real application playback, audible continuity,
packaged installation, physical-device timing or Windows/macOS hardware
behavior. Those observations remain deferred and unpassed in the manual epic.

## Story acceptance-criterion map

| Story criterion | Deterministic evidence | Manual evidence still required |
| --- | --- | --- |
| Protocol additions ship in the coordinator, Windows and Swift codecs with golden and compatibility coverage | `TestPhaseOneTransmissionGoldenSetIsComplete`, coordinator and Windows `TestGoldenRoundTrip`, Windows `TestMirrorMatchesCoordinatorSource`, Swift `ProtocolContractTests.roundTripEveryGolden`, and all three legacy-voice compatibility tests | None for wire compatibility |
| Ready targets use the exact three-second barrier and one start T derived from fresh maximum RTT | `TestTransmissionSchedulerEnforcesDomainFIFOAndExactRTTBarrier`, `TestTransmissionSchedulerPartialReadinessAndNoLateAutoplay`, `TestOverlayControllerRuntimeSendsExactPrepareAndRTTSchedule`, and `TestOverlayControllerMultiTargetBarrierUsesFreshMaximumRTTAndOneT` | `TASK-260712-2hodti` must measure real ready-target start skew on exact Windows/macOS builds and physical outputs; it is deferred and has not run |
| Late, offline and DND targets receive exact non-autoplay outcomes | `TestTransmissionSchedulerPartialReadinessAndNoLateAutoplay`, `TestTransmissionSchedulerRechecksBlockDNDOfflineAndClockEvidence`, `TestTransmissionSchedulerRechecksDNDWithoutSuppressingUserMessagesOnly`, Windows `TestMediaClipExpiredAndLatePrepareNeverFetchOrReportReady` and `TestMediaClipScheduleRejectsStaleUnsyncedAndUnadvertisedStarts`, and Swift `expiredPrepareNeverFetchesOrReportsReady`, `missedPrepareDeadlineIsLeftForCoordinatorTimeout`, and `staleScheduleNeverArms` | `TASK-260712-2hodti` must confirm that stale/disarmed work is inaudible in the real applications; it is deferred and has not run |
| Overlay and interrupt work never overlaps within one effective playback domain | `TestTransmissionSchedulerEnforcesDomainFIFOAndExactRTTBarrier`, `TestTransmissionSchedulerBreaksEqualAcceptanceTiesByULIDAcrossDeliveries`, `TestTransmissionSchedulerSerializesOppositeApproachOrigins`, and `TestOverlayControllerMainPauseAndSkipDoNotTerminateScheduler` | `TASK-260712-2hodti` must confirm audible non-overlap and main-program continuity on real outputs; it is deferred and has not run |
| Ordering follows coordinator acceptance time and cannot be caller-controlled | `TestTransmissionHTTPStrictJSONAndStableErrors` rejects caller `accepted_at` and `transmission_id`; `TestCreateTransmissionPersistsImmutableOnlineAndOfflineSnapshots` protects the immutable key; FIFO, equal-time ULID and opposite-origin scheduler tests prove the trusted order | None |
| Direct or copied media IDs cannot bypass target ACL or tenant boundaries | `TestTransmissionHTTPStrictJSONAndStableErrors` covers foreign media and outside audiences; `TestTransmissionTargetSnapshotIsTheOnlyGenericMediaACL`, `TestResolvedTransmissionSealsMixedAudienceIdempotencyVisibilityAndCancel`, and the generic media download tests cover exact binding, block, revoke and replacement cases | None |
| A mixed fleet downgrades the whole overlay visibly to `after_current` without splitting protocols | `TestTransmissionHTTPWholeDowngradeAndExplicitInterruptConfirmation`, `TestOverlayControllerWholeDowngradeNeverSplitsTargetProtocols`, `TestOverlayControllerLegacyBridgeUsesGenericACLAndExactTargets`, and the three legacy codec compatibility tests | Later UI wording is exercised by its own UI/manual tasks; the transmission API and runtime contract are fully deterministic here |

## Adversarial regression map

| Invariant | Automated proof |
| --- | --- |
| Immutable audience, copied IDs, cross-orbit access and authorization views | `TestCreateTransmissionPersistsImmutableOnlineAndOfflineSnapshots`, `TestTransmissionTargetSnapshotIsTheOnlyGenericMediaACL`, `TestResolvedTransmissionSealsMixedAudienceIdempotencyVisibilityAndCancel`, `TestTransmissionHTTPVisibilityAndStartedCancelConflict` |
| Origin defaults, selector deduplication and local-only DND bypass | `TestTransmissionIncludeOriginDefaultsAreClosed`, `TestResolvedTransmissionExplicitSelectorsDeduplicateThenFilterOrigin`, `TestResolvedTransmissionAppliesDNDAndBypassesItOnlyForLocalThisPulsar` |
| Clip/delivery validation and the exact 60-second overlay boundary | `TestTransmissionHTTPStrictJSONAndStableErrors` covers disabled delivery, track mismatch, 60,000 ms acceptance and 60,001 ms rejection |
| Idempotency, confirmation replay and visible whole-delivery downgrade | `TestResolvedTransmissionConcurrentIdempotencyHasOneAcceptance`, `TestResolvedTransmissionInterruptConfirmationIsExplicitBoundAndSingleUse`, `TestTransmissionHTTPWholeDowngradeAndExplicitInterruptConfirmation`, `TestOverlayControllerWholeDowngradeNeverSplitsTargetProtocols` |
| FIFO, equal-time ties, overlay/interrupt serialization and two opposite approach origins | `TestTransmissionSchedulerEnforcesDomainFIFOAndExactRTTBarrier`, `TestTransmissionSchedulerBreaksEqualAcceptanceTiesByULIDAcrossDeliveries`, `TestTransmissionSchedulerSerializesOppositeApproachOrigins` |
| Exact barrier, max-RTT T, partial readiness, no-ready and stale-play handling | `TestOverlayControllerMultiTargetBarrierUsesFreshMaximumRTTAndOneT`, `TestTransmissionSchedulerPartialReadinessAndNoLateAutoplay`, `TestTransmissionSchedulerNeverSchedulesAtOrPastDeliveryExpiry`, Windows/Swift client deadline tests |
| Closed receipt vocabulary | `TestTransmissionReceiptVocabularyIsClosedAndPersistent` persists every one of the 35 valid terminal status/reason pairs; targeted runtime tests exercise policy precedence, authenticated binding and generation races |
| Block, DND, offline, capability and clock rechecks | `TestTransmissionSchedulerRechecksBlockDNDOfflineAndClockEvidence`, `TestTransmissionSchedulerRechecksDNDWithoutSuppressingUserMessagesOnly`, `TestTransmissionSchedulerActiveDNDUsesFadeStopCancellation`, `TestOverlayControllerReconnectNeverResendsAfterCapabilityLoss` |
| Sender cancel, delete/expiry, target revoke, leave/apart and acknowledgement timeout | `TestCancelAuthorizedTransmissionReturnsGenerationBoundDisarm`, `TestTransmissionSchedulerDeleteCancellationAndAckTimeoutConverge`, `TestTransmissionSchedulerDeliveryExpiryDisarmsPreparedWork`, `TestTransmissionSchedulerApproachSplitDisarmsOnlyNonStartedTargets`, `TestOverlayControllerCancellationUsesExactDisarmAndActiveInterruptFade`, `TestOverlayControllerRevokedSocketCanAckButReplacementCannot` |
| Restart, reconnect and timer cleanup | `TestTransmissionSchedulerRestartCancelsPreparedKeepsDeadlineAndStalesPast`, `TestOverlayControllerReconnectResendsFutureGenerationWithoutNewAcceptance`, `TestOverlayControllerTimerNeverExtendsAnArmedDeadline`, `TestOverlayControllerTerminalWorkClearsTimerAndStaleWakeIsInert` |
| Legacy exact targets and no invented interrupt fallback | `TestTransmissionSchedulerLegacyClaimIsDurableAndIdempotent`, `TestOverlayControllerLegacyBridgeUsesGenericACLAndExactTargets`, `TestOverlayControllerWholeDowngradeNeverSplitsTargetProtocols`, `TestOverlayControllerNeverInventsInterruptFallback` |

## Additive database proof

- `TestTransmissionSchedulerSchemaBackfillsPredecessorAcceptanceOnly` models a
  row written before the scheduler companion and proves that reopening creates
  only an acceptance-time companion, never an invented barrier or schedule.
- `TestTransmissionStoreExactPreviousHeadRollback/pre_scheduler_companion`
  runs the exact `0c1e1946ff692aa553c19ca6bf7328150d1a24b8` coordinator against
  the current database and then verifies transmission rows, target rows, ACL,
  legacy state and scheduler state byte-for-byte after roll-forward.
- `TestTransmissionStoreExactPreviousHeadRollback/pre_transmission_schema`
  repeats the rollback with exact revision
  `2aa97c2d08cb93b110200ae159fd43265410ff5a`, from before transmission tables
  existed, proving that the entire schema remains additive to that boundary.
- `TestTransmissionSchemaInstallIsAtomic` and foreign-key checks retain the
  failure-atomic and referential-integrity guards.

## Accepted engineering verification

- Coordinator vet, full unit suite and full race suite: passed locally.
- Focused scheduler/store/protocol suite with shuffled order for 20 runs:
  passed locally.
- Both exact previous-version rollback subtests: passed locally.
- Windows vet, unit and race suites: passed locally.
- Swift release build: passed locally. The workstation still cannot compile
  the repository's existing `Testing`-based test target; hosted macOS
  `node-core` passed the full Swift test suite on the exact code commit.
- Hosted run `29333494719` passed coordinator, node-core, pulsar-win and the
  signed packaged-probe job on exact code head `c60bd99`.
- Real-app and real-hardware rows: not run, not inferred and not claimed.
