# P1 media ingest automated acceptance evidence

Date: 2026-07-14
Task: `TASK-260712-3huupe`
Story: `STORY-260712-ld674h`

This file maps the phase-one ingest acceptance contract to deterministic tests.
It is engineering evidence only: synthetic ffmpeg fixtures, unit tests,
store-backed integration tests and in-process HTTP tests do not claim a real
application, physical device, human listening result or production soak.

## Story acceptance map

| Acceptance surface | Automated evidence |
| --- | --- |
| Every accepted container and codec reaches canonical WAV through the common service | `TestSubmitUploadLiveSupportedFormatAcceptanceMatrix` processes WAV/PCM, MP3, M4A/AAC, M4A/ALAC, ADTS AAC, OGG/Opus, OGG/Vorbis and FLAC through `SubmitUpload`/`SubmitMedia`; `TestProcessorLiveSupportedFormatMatrix` independently pins the worker result for the same matrix. |
| App upload runs end to end | `TestMediaIngestAcceptanceHTTPUploadACLDeleteCleanup` proves authenticated create, idempotent replay, two-chunk resume, finalization, canonical publication, owner and snapshotted-target reads, foreign non-disclosure, immediate delete revocation and asynchronous byte cleanup. |
| Corrupt, truncated, polyglot, protocol-shaped and oversized input never becomes ready | `TestSubmitMediaFailuresRemainNonReadyAndKeepSourceForRetention`, `TestSubmitMediaDeclaredLengthMismatchNeverInvokesWorker`, `TestSubmitMediaRejectsSparseOversizedSourceBeforeWorker` and `TestSignatureProbeRejectsUnsupportedTruncatedAndPolyglotBeforeWorker` pin terminal sanitized failures, empty publication metadata and retained private source bytes. The HLS/HTTPS-shaped fixture is rejected before a worker is invoked. |
| Duration/decompression pressure, probe and transcode failure remain bounded | `TestSubmitUploadLiveRejectsCompressedDurationBomb` sends a compact 181-second AAC fixture through the common service and proves rejection before publication. `TestSubmitMediaFailuresRemainNonReadyAndKeepSourceForRetention` covers excessive reported duration, stream-layout and codec mismatch, ffprobe timeout/crash, ffmpeg timeout/crash, invalid loudness, invalid canonical bytes and output-size cap. `TestProcessorCanonicalizesWithFixedNetworkDisabledResourceCappedCommands`, `TestLinuxWorkerStartsBehindKernelLimitBarrier` and `TestLinuxWorkerKernelFileCapStopsOversizedOutput` pin file-only protocols, fixed arguments, wall limits and Linux kernel limits. |
| Retry and interruption do not duplicate media or publication | `TestMediaUploadHTTPCreateResumeFinalizeAndReplay`, `TestMediaUploadHTTPConcurrentSameOffsetHasOneWriter`, `TestMediaUploadHTTPRestartReconcilesCrashTail`, `TestSubmitUploadConcurrentRetryRunsOnePublication`, `TestSubmitUploadRecoversCrashAfterAtomicPublishBeforeCAS` and `TestMediaIngestInterruptedPublicationIsRecoverable`. |
| Quotas are transactional and idempotent replay is free | `TestMediaUploadHTTPStableFailuresAndQuota`, `TestAuthorizedMediaUploadQuotaBoundariesAndReplay`, `TestAuthorizedMediaUploadConcurrentIdempotencyHasOneResultAndToken` and `TestAuthorizedMediaUploadConcurrentQuotaHasOneReservation`. |
| Dedupe and authorization are tenant-scoped and non-disclosing | `TestSubmitMediaDedupeIsPhysicalWithinOrbitAndAbsentAcrossOrbits`, `TestMediaIngestAcceptanceHTTPUploadACLDeleteCleanup`, `TestMediaDownloadHTTPEnforcesOwnerAndExactTargetACL`, `TestDownloadServiceUsesExactTargetSnapshotAndOwnerControl` and `TestAuthorizeMediaDownloadSeparatesOwnerControlAndSnapshottedNodes`. |
| Delete, expiry and stale workers converge without resurrecting bytes | `TestDeleteDuringCanonicalPublicationCannotLeaveOrphanBytes`, `TestMediaIngestStalePublicationCannotReviveDeletedMedia`, `TestMediaDeleteExpiryRaceHasOneTerminalPolicy`, `TestLifecycleExpiresReadyClipAtSevenDayBoundary` and `TestLifecycleCleanupCrashRetryConvergesFromMissingFile`. The last test reopens SQLite and creates a new lifecycle service after the simulated unlink-before-receipt crash. |
| Telegram keeps common processing and legacy ordering/playback | `TestTelegramAdapterUsesSubmitMediaAndReturnsLegacyCompatibilityWAV`, `TestTelegramAdapterPreservesCommonFailureCode`, `TestTelegramVoiceSubmitMediaAdapterPreservesFIFORepliesAndLegacyPlayback`, `TestTelegramVoiceCommonFailureKeepsBotReplyAndBothStatusesTerminal`, `TestLifecycleDeleteRevokesAndCleansLinkedTelegramCompatibilityBytes` and `TestLifecycleTelegramSourceCleanupRetriesAfterUnlinkCrash`. |
| Mixed-version rollback remains writable and authoritative | The `previoushead` build-tag suite runs `TestMediaIngestExactPreviousHeadRollback`, `TestMediaUploadExactPreviousHeadRollback`, `TestMediaProcessingExactPreviousHeadRollback`, `TestMediaLifecycleExactPreviousHeadRollback` and `TestMediaIntegrationExactPreviousHeadRollback` against their pinned immediate predecessors. |

## Security and failure invariants

- No fixture supplies a worker command, URL, demuxer, output path or filter.
  Worker arguments contain server-owned local paths, `file` as the only
  protocol allowlist and the explicit network denylist.
- Signature, declared-length and hard-size failures occur before ffprobe or
  ffmpeg. Later worker failures persist only a stable code; worker diagnostics,
  credentials, titles and local paths are absent from the HTTP body and logs.
- A media item becomes downloadable only after the canonical file is fsynced
  and the publication transaction commits. Failed, deleted and expired rows
  have no visible storage key.
- Delete authorization revokes reads in the same transaction. Physical cleanup
  and delivery cancellation are durable, retryable outbox work.
- Same-orbit canonical dedupe may reuse an inode behind distinct opaque keys;
  identical bytes in another orbit do not share a physical file or disclose a
  match.

## Reproducible gates

From `coordinator/`:

```sh
go vet ./...
go test ./...
go test -race ./...
go test -tags previoushead -count=1 ./internal/store -run '^(TestR8ExactPreviousHEAD(AuthorityRoundTrip|TwoGenerationProjectionComposition|ConfigBootstrapContract)|TestMediaIngestExactPreviousHeadRollback|TestMediaUploadExactPreviousHeadRollback|TestMediaProcessingExactPreviousHeadRollback|TestMediaLifecycleExactPreviousHeadRollback|TestMediaIntegrationExactPreviousHeadRollback)$'
```

The live format tests require `ffmpeg` and `ffprobe`; they skip when those tools
are absent. Hosted coordinator CI installs ffmpeg, so a green hosted run is the
authoritative result for the full synthetic codec matrix.

## Explicitly unclaimed evidence

The automated gate does not prove real microphone capture, speaker quality,
hardware audio-session interaction, Store packaging behavior, real network
interruption or human-perceived loudness. P1 real-app and physical-platform
acceptance remains deferred to `EPIC-260714-th54l3`, principally
`TASK-260712-1vtwkl`, `TASK-260712-2hodti` and `TASK-260712-e5mfqj`. A failure
there reopens the relevant engineering task; it is not converted into a pass by
this synthetic matrix.
