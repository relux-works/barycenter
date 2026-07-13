# Implement SubmitMedia validation and canonical WAV pipeline

## Description
Create the common server-side SubmitMedia service used by app uploads and Telegram so every phase-one clip follows the same authoritative validation, normalization and status lifecycle.

## Scope
Implement transport-neutral SubmitMedia for persisted app uploads and Telegram downloads. Validate requested clip kind, actual byte limit and magic signature, run ffprobe with strict timeout and resource limits, compute hashes and tenant-only dedupe, then execute the exact high-pass, compressor and loudnorm chain in a constrained ffmpeg worker with network protocols disabled, fixed non-user-derived arguments, CPU, memory and output-size caps. Write PCM s16le 44.1-kHz stereo canonical WAV to a temporary storage key and publish ready metadata atomically only after probe, transcode, hash, loudness and storage succeed. Delete source bytes after success; keep failures nonready under failed retention.

## Acceptance Criteria
Only genuinely supported WAV, MP3, M4A or AAC, OGG or Opus and FLAC clips within 180 seconds and 50 MiB become ready. Corrupt, polyglot, truncated, decompression-bomb, timeout, worker-crash, oversized-output and unsupported inputs cannot escape the worker, access network, become downloadable or leave partial ready bytes. Idempotent retry returns the same tenant result, cross-orbit hashes reveal nothing, and output metadata plus atomic storage match the canonical contract.
