# P1 SubmitMedia processing contract

`TASK-260712-2af2dp` introduces one transport-neutral server entry point for
phase-one clip processing. App uploads call `SubmitUpload`, which resolves the
persisted upload session and then enters `SubmitMedia`. Telegram can enter the
same service with a persisted `processing` media item and a server-owned source
path; switching the bot adapter itself remains scoped to `TASK-260712-12ojcb`.

This is engineering evidence only. It does not claim real-app, physical-device
or listening acceptance; those claims are tracked in `EPIC-260714-th54l3`.

## Accepted input and canonical output

- Actual input size is positive and at most 50 MiB; declared and copied sizes
  must match exactly.
- Duration is positive and at most 180,000 ms.
- Supported containers/codecs are WAV/PCM, MP3, M4A/AAC or ALAC, ADTS AAC,
  OGG/Opus or Vorbis, and FLAC.
- A bounded signature/container check runs before `ffprobe`; a fixed demuxer
  then prevents format auto-detection from selecting a different input path.
- The default filter chain is exactly
  `highpass=f=90,acompressor=threshold=-20dB:ratio=3:attack=10:release=180:makeup=4,loudnorm=I=-14:TP=-1.5:LRA=11:print_format=json`.
- Canonical output is one PCM `s16le`, 44.1-kHz, stereo WAV. The stored
  metadata includes duration, byte size, SHA-256 and the bounded loudnorm JSON
  containing source and output integrated loudness and true peak.

## Worker boundary

Arguments, filters, demuxers and paths occupy fixed argument positions and are
never interpreted by a shell. Both tools receive a file-only protocol
allowlist plus an explicit network/protocol denylist, one worker thread,
bounded stdout/stderr capture and independent deadlines. Production Linux
starts each tool behind a parent-controlled barrier and applies kernel
`RLIMIT_CPU`, `RLIMIT_AS`, `RLIMIT_NOFILE` and `RLIMIT_FSIZE` before releasing
the process to `exec`. The default limits are 45 CPU seconds, 512 MiB address
space, 64 descriptors and 34 MiB output; wall deadlines are 10 seconds for
probe and 60 seconds for transcode. One service admits at most four media
workers at once; a five-second saturated-capacity wait remains retryable and
does not terminally fail the media row.

Non-Linux coordinator development builds retain the fixed arguments, protocol
policy, capture bounds and wall deadlines. The production coordinator remains
Linux-only for the kernel resource-limit guarantee.

## Publication, retry and dedupe

1. Source bytes are copied into a mode-0600 private work directory with an
   exact size check. App submissions must match the persisted media ID,
   session ID, declared length and service-owned `.uploads/<session>.part`
   path before any worker is admitted.
2. Probe, transcode, output probe, loudness validation and SHA-256 complete
   before repository publication begins.
3. The canonical staging file is mode 0600 and fsynced.
4. The repository stages one random opaque `media/v1/<64 hex>` publish
   operation with a media revision CAS.
5. A hard link atomically publishes the canonical file, followed by file and
   directory fsync.
6. One transaction changes the item to `ready`, records all metadata, completes
   the upload session and marks the storage operation done.
7. Only after that commit are source bytes removed. App-upload cleanup has a
   durable `temp_cleaned_at` acknowledgement and the existing maintenance pass
   retries an interrupted delete.

A crash after the hard link but before the database CAS leaves the item
non-ready and the publish operation pending. A retry reuses the same storage
key, verifies and fsyncs the existing canonical bytes, then completes the CAS.
Completed retries return the same media row without invoking workers again.
Private `.processing` and `.canonical-*` artifacts are removed on service
restart; published opaque canonical names are preserved.

Canonical-byte dedupe queries always require `owner_orbit_id`. Identical ready
bytes inside one orbit use separate opaque storage keys backed by the same
inode, so lifecycle operations remain independent. No query or physical reuse
crosses orbit boundaries.

## Failure contract

Signature/container rejection, unsupported codec/layout, invalid or excessive
duration, size mismatch, probe timeout/failure, worker timeout/crash, invalid
loudness, missing or oversized output and invalid canonical output are stable
sanitized processing failures. They transition the media item and any upload
session to `failed`; no storage key, publication timestamp or downloadable
state is set. The HTTP surface collapses these internal codes to
`422 media_processing_failed`. Infrastructure failures remain retryable in
`processing`/`finalizing` and are logged only by fixed operation class, without
paths, credentials, request titles or worker diagnostics.

## Automated gates

- Unit and store-backed integration coverage exercises all state transitions,
  unsupported/corrupt/polyglot/truncated/oversized/duration/timeout/crash/output
  failures, idempotent and concurrent retry, atomic-publish crash recovery,
  source cleanup and tenant isolation.
- HTTP tests prove final-chunk processing, stable redaction and recovery of an
  already-finalizing upload after an interrupted processor.
- Hosted Linux CI installs ffmpeg and runs a synthetic live matrix for WAV,
  MP3, M4A/AAC, ADTS AAC, OGG/Opus and FLAC, plus the legacy live Opus path and
  kernel-limit tests.
- The tagged rollback gate executes the exact immediate predecessor
  `050c9792e328730e33bb65cf03fcda8e3d690061` against a database containing a
  ready canonical row, then rolls forward and verifies ready metadata, dedupe
  lookup, predecessor writes and foreign keys.
