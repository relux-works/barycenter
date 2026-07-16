# P2 audio-track intake and codec no-go pipeline

Task: `TASK-260712-285pag`

## Outcome

The common app upload endpoint now accepts `audio_track` only after current
content-policy consent. The declared and actual input ceiling is 500 MiB and
the constrained probe ceiling is two hours. Existing actor/orbit upload,
input, retained-storage, temporary-processing and concurrent-job quotas remain
authoritative.

The accepted codec handoff, `p2-codec-player-adr-handoff.v1`, selects no
production codec or player. Consequently a valid track finishes with the
stable sanitized failure `codec_profile_unavailable`. This is intentional: the
pipeline does not manufacture a candidate profile, mark a variant ready, make
the original upload a decoder input, or enable `audio_track_v1`. A reviewed
replacement ADR must change the persisted policy and add an explicit encoder
registry before production generation can be implemented.

## Validation and metadata

The coordinator copies exactly the declared bytes into a private `0700` work
directory with a 500 MiB hard limit. It performs only a fixed-prefix signature
classification locally. Full container probing is delegated to the existing
worker boundary with:

- file-only protocol allow-list and the complete network protocol deny-list;
- one worker thread, bounded probe output/logs, CPU, address space and open
  file limits, plus the existing wall deadline and global worker queue;
- exactly one audio stream, an approved input container/codec pair, at most
  eight channels, at most 384 kHz and at most 7,200,000 ms;
- SHA-256 over at most 500 MiB of exact copied input.

Successful validation persists immutable original filename, MIME, container,
codec, byte size, SHA-256, duration, sample rate and channel count in
`stream_track_metadata`. Duration/sample/channel columns are additive with
zero defaults so the exact previous coordinator can roll back and forward
without losing rows it does not understand.

## Processing and cleanup

After metadata admission, `stream_processing_jobs` reserves the exact copied
input size as temporary storage, records the current/high-water value and
closes the lease as `processor_failed` when the empty production registry is
confirmed. Work directories are removed on every return path. No generated
PCM WAV, compressed candidate, seek map, chunk manifest, storage key or staged
variant is created.

The generic media terminal transaction records the stable failure and moves
the upload session to `failed`. Failed source bytes remain under the existing
failed-upload cleanup contract (maintenance removes them within its 24-hour
maximum); no separate track retention bypass exists. Retry returns the same
failure without invoking the worker again. Delete/expiry and worker failure
cannot expose partial media because production publication is unreachable
while the codec policy is locked.

Phase 1 clip and Telegram voice processing are unchanged: they still use the
separate WAV canonicalization path and its explicit high-pass, compressor and
loudness filters. The track probe never calls FFmpeg transcoding and contains
none of those filters.

## Evidence

The deterministic tests cover the two-hour and 500 MiB boundaries, current
consent, probed metadata, no-go replay, worker crash, pre-worker oversized
input, network/resource command bounds, absence of speech filters, absence of
canonical/temp artifacts, no variants, Phase 1 regression, race execution and
exact-predecessor rollback:

```sh
cd coordinator
go test ./internal/media -run 'Test(TrackProbe|SubmitAudioTrack)' -count=1
go test -race ./internal/media -run 'Test(TrackProbe|SubmitAudioTrack)' -count=1
go test ./cmd/duet-coordinator -run TestMediaUploadHTTPAudioTrack -count=1
go test ./internal/store -run 'TestStream(Track|Accounting)' -count=1
go test -tags previoushead ./internal/store -run TestMediaIngestExactPreviousHeadRollback -count=1
```

The complete hosted coordinator suite remains the authoritative environment
for the OGG/Vorbis fixture because the local FFmpeg package does not include
the `libvorbis` encoder.
