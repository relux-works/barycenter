## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-12T16:22:53Z

## Blocked By
- TASK-260712-dqdoqj
- TASK-260712-1vdlkw
- TASK-260712-3vkcki
- TASK-260712-298tyq
- TASK-260712-1canzv
- TASK-260712-350u8d

## Blocks
- TASK-260712-2eympi

## Checklist
- [ ] Run the two-hour RSS, seek-to-audio, and scheduled-start skew matrix on the same fixtures for every viable candidate
- [ ] Cover pause, resume, repeated seek, cache reuse, and range failure behavior on both platforms
- [ ] Compare pure-Go, Media Foundation, and bundled paths side by side with identical artifact formats
- [ ] Capture mixed Windows macOS timing behavior and any format-specific caveats
- [ ] Include concrete failure evidence for rejected options rather than prose-only conclusions
- [ ] Compare complete Windows plus macOS combinations across Windows-Windows, Windows-macOS and macOS-macOS
- [ ] Require at least one combination to pass every hard gate without score averaging

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p2-codec-player-spike-sequence.puml](file://TASK-260712-ibuaxj/p2-codec-player-spike-sequence.puml) — Shared proof sequence for comparative start, seek, and memory evidence
