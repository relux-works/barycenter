## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-12T16:19:32Z

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
- [ ] Introduce a common SubmitMedia entry point shared by app and Telegram sources
- [ ] Add signature probe, ffprobe, ffmpeg, hash and dedupe processing with capped resources
- [ ] Cover corrupt, unsupported, oversized and timeout failure states so non-ready media never leaks
- [ ] Run ffprobe and ffmpeg with network disabled, fixed arguments and strict CPU, memory, time and output caps
- [ ] Publish canonical storage atomically only after every validation and metadata step

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-sequence.puml](file://TASK-260712-2af2dp/p1-media-ingest-sequence.puml) — Common SubmitMedia validation and canonicalization flow
