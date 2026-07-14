## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-14T03:04:22Z

## Blocked By
- TASK-260712-z6h6wh

## Blocks
- TASK-260712-gj0cko
- TASK-260712-12ojcb
- TASK-260712-3huupe
- TASK-260712-3mcof4
- TASK-260712-1sae4q
- TASK-260712-2qpp6w
- TASK-260712-3dmllz
- TASK-260712-285pag

## Checklist
- [x] Introduce a common SubmitMedia entry point shared by app and Telegram sources
- [x] Add signature probe, ffprobe, ffmpeg, hash and dedupe processing with capped resources
- [x] Cover corrupt, unsupported, oversized and timeout failure states so non-ready media never leaks
- [x] Run ffprobe and ffmpeg with network disabled, fixed arguments and strict CPU, memory, time and output caps
- [x] Publish canonical storage atomically only after every validation and metadata step

## Notes
Strict inline execution started 2026-07-14 from merged main 050c9792e328730e33bb65cf03fcda8e3d690061 on branch task/task-260712-2af2dp-submitmedia-processing-pipeline. TASK-260712-1bnos4 is landed; no real-app or physical-hardware acceptance is claimed in this engineering task.
Implementation complete pending commit and hosted CI: shared SubmitMedia plus app finalization, exact-size/signature/container/ffprobe validation, fixed network-disabled ffmpeg, Linux kernel rlimits, fsynced atomic hard-link plus CAS publication, tenant-only dedupe, sanitized failed lifecycle, restart cleanup and finalizing retry recovery. Local full Go tests, race, vet, focused x20 stress, pulsar-win tests/cross-build, board validation and exact 050c979 predecessor rollback pass. Local Swift remains environment-limited by the missing Testing module; hosted macOS and hosted Linux ffmpeg/rlimit gates are required. No real-app or physical-hardware evidence is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-sequence.puml](file://TASK-260712-2af2dp/p1-media-ingest-sequence.puml) — Common SubmitMedia validation and canonicalization flow
- [p1-submitmedia-processing-contract.md](file://TASK-260712-2af2dp/p1-submitmedia-processing-contract.md) — Shared SubmitMedia validation, constrained worker, atomic publication, retry, dedupe and rollback contract
