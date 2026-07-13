# Implement secure audio_track ingest and canonical variants

## Description
Extend SubmitMedia with the exact ADR pipeline for long user audio while preserving track dynamics unless the reviewed profile explicitly says otherwise.

## Scope
Accept requested_kind audio_track only with current content-policy consent and enforce actual input at most two hours and 500 MiB plus configured orbit quotas. Probe signatures and duration in the constrained worker, extract metadata, and generate the exact compressed variants, seek and integrity metadata chosen by the ADR without persisting a full PCM WAV or accidentally applying the Phase 1 speech high-pass, compressor and loudnorm chain to music. Bound temporary disk, CPU, memory, output and time; disable network protocols; publish variants atomically; report processing progress; and make retries, worker crash, deletion and cleanup idempotent.

## Acceptance Criteria
One-hour and maximum-bound inputs produce only the pinned, integrity-verifiable ready variants and honest metadata or a stable failure. Missing consent, corrupt, oversized, unsupported, quota, worker crash and delete races never publish partial media. Original and temporary bytes follow retention, no full WAV persists, clip and Telegram voice pipelines remain green and track audio is not destructively speech-normalized without an explicit ADR decision.
