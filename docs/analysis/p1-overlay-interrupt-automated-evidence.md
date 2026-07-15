# P1 overlay and interrupt automated evidence

This packet is the engineering gate for `TASK-260712-3d6cnn`. It records only
deterministic tests and source-level safety checks. It does not claim audible,
route, Store-package, or physical-device acceptance.

## A3 overlay mapping

| Requirement | Windows evidence | macOS evidence | Automated bound |
| --- | --- | --- | --- |
| Scheduled start and pre-duck | `TestWindowsOverlayContinuouslyConsumesMainAndLimitsFinalMix` | `preDuckStartsAtTMinus250AndDisposeRestoresReusableGraph`, `tenSecondOverlayUsesFrozenGainOrderAndLeavesGraphReusable` | first-sample report within 200 ms; 250 ms raised-cosine attack or deterministic late catch-up |
| Main continuity and gain order | `TestWindowsOverlayContinuouslyConsumesMainAndLimitsFinalMix`, `TestWindowsOverlayKeepsContinuityAcrossRepeatedMainHandoffs` | `tenSecondOverlayUsesFrozenGainOrderAndLeavesGraphReusable`, `overlayGraphGainOrder` | main ring remains consumed; main + clip -> limiter -> local master |
| Limiter ceiling and hit counter | `TestWindowsOverlayContinuouslyConsumesMainAndLimitsFinalMix` | `tenSecondOverlayUsesFrozenGainOrderAndLeavesGraphReusable`, `overlayGraphGainOrder` | Windows output is capped at -1 dBFS; macOS limiter threshold/headroom is -1.1/+0.1 dB and hit windows increment |
| Cancel and sender delete | `TestWindowsOverlayCancellationFadesAndReleasesDuckBeforeAck`, `TestWindowsSenderDeleteDuringOverlayRestoresMainProgram` | `cancellationFadesOverlayReleasesDuckAndAcknowledgesOnce` | 120 ms clip fade, 600 ms duck release, one acknowledgement, main returns to unity |
| Repeated operation | `TestWindowsRuns100SequentialOverlaysWithoutGrowthOrDeadlock` | `macOSRuns100SequentialOverlaysWithoutRetainedClipOwners` | 100 sequential overlays, no deadlock, stale active graph, retained clip owner, or >4 MiB retained Windows heap growth |

## A4 interrupt mapping

| Requirement | Windows evidence | macOS evidence | Automated bound |
| --- | --- | --- | --- |
| Freeze/replace at T | `TestWindowsInterruptRenderFreezesMainAtTAndReplacesIt` | `interruptFadesAtTMinus250ResumesExactAnchorOnceAndReusesGraph` | deterministic start report within 500 ms and no overlay fallback |
| Audible anchor | `TestWindowsInterruptResumesOnceFromAudibleAnchorWithFadeIn` | `audibleInterruptAnchorSubtractsQueuedRingTail`, `interruptFadesAtTMinus250ResumesExactAnchorOnceAndReusesGraph` | captured provider position minus queued ring duration; clamp at zero |
| Resume and cancellation | `TestWindowsInterruptResumesOnceFromAudibleAnchorWithFadeIn`, `TestWindowsInterruptCancelFadesThenResumesAndAcknowledgesOnce`, `TestWindowsInterruptCancellationHonorsResumeMainFalse`, `TestWindowsInterruptCancelDuringNaturalResumeAcknowledgesCachedResultOnce` | `activeInterruptCancelFadesAndAcknowledgesOneResume`, `cancelDuringResumeProducesOneCancelledTerminalState` | one seek/resume or explicit abandon at the exact anchor, 120 ms fade-in, one cached terminal outcome |
| Stale work and failure | `TestWindowsInterruptStopInvalidatesOldResumeToken`, `TestWindowsInterruptNaturalResumeFailureIsFailedNotEnded`, `TestMediaClipAsyncMixerFailureIsTypedAndNeverReportedAsEnded` | `reconnectResetCancelsInterruptAndRejectsLateCallbacks`, `interruptResumeFailureIsDistinctAndLeavesGraphReusable` | old generation/timer cannot resume; async graph/provider failure is typed and never reported as end success |

## Realtime and memory guards

- Windows `TestRenderBoundaryHasNoBlockingOrDynamicOperations` rejects
  goroutines, allocation helpers, locks, waits, and sleeps from `Render`,
  `mixOverlay`, `mixInterrupt`, limiter, and gain functions.
- Windows `TestWindowsOverlayRenderAllocationsStayZero` measures zero allocations
  across the active ring/duck/clip/limiter render path.
- macOS `renderCallbackSourceSafety` rejects dispatch, locks, waits, allocation,
  file/network I/O, and sleeps inside the marked render callback, and proves the
  source ring is consumed independently of overlay state.
- macOS `renderControlPublicationSafety` requires atomic reader ownership,
  serializes multiple gain-command producers outside the callback, and keeps
  FIFO idle interruptible so shutdown cannot depend on a writer connecting.
- macOS `playerStateSnapshotSafety` requires heartbeat playback, anchor and
  speaker fields to be read as one PlayerCore queue-owned snapshot.
- `TestWindowsMaximumP1ClipHasOneBoundedDecodedBuffer` and
  `maximumP1ClipUsesOneBoundedPreparedPCMBuffer` enforce one prepared PCM buffer
  for the 180-second P1 maximum: 7,938,000 stereo frames, 63,504,000 bytes,
  below 64 MiB. Scheduling must retain the same backing buffer, not copy it.
- Windows and macOS client validation rejects a duration above 180 seconds
  before fetch or decode.

## Hardware-only remainder

The following evidence remains manual under `EPIC-260714-th54l3`, principally
`TASK-260712-2hodti`, and is not satisfied by this packet:

- listening for clicks, pumping, audible clipping, speech masking, or route noise;
- measuring real Spotify/provider audible position after interrupt within 500 ms;
- running 100 real clips against packaged Windows and signed macOS builds;
- Bluetooth, USB, AirPlay/Airfoil, default-route changes, sleep/wake, and device loss;
- physical Windows 10/11 and supported macOS hardware/OS matrices.

The automated packet may gate best-effort engineering merge. A3/A4 product
acceptance still requires the separate manual epic.
