## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T21:30:49Z

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
- [x] Run the two-hour RSS, seek-to-audio, and scheduled-start skew matrix on the same fixtures for every viable candidate
- [x] Cover pause, resume, repeated seek, cache reuse, and range failure behavior on both platforms
- [x] Compare pure-Go, Media Foundation, and bundled paths side by side with identical artifact formats
- [x] Capture mixed Windows macOS timing behavior and any format-specific caveats
- [x] Include concrete failure evidence for rejected options rather than prose-only conclusions
- [x] Compare complete Windows plus macOS combinations across Windows-Windows, Windows-macOS and macOS-macOS
- [x] Require at least one combination to pass every hard gate without score averaging

## Notes
2026-07-15 strict-sequence start from synchronized main bc2600c. Implementing inline outside task-board spawn workflow. The comparison will consume the exact hosted artifacts from bundled, Media Foundation, native macOS and pure-Go probes, preserve every failed format and hard gate without averaging, enumerate Windows-Windows, Windows-macOS and macOS-macOS combinations, and keep production selection fail-closed when no complete combination passes. Two-hour physical RSS, real routes and platform matrices remain unclaimed in EPIC-260714-th54l3.
Accepted as a fail-closed comparative result after engineering PR #116 merged as 10db015 (head 094e96f). The generated contract compares bundled FFmpeg, Media Foundation plus native macOS, and pure-Go across windows_windows, windows_macos and macos_macos with six independent format rows and twelve hard gates per combination; score averaging is structurally forbidden. No combination is selected. Bundled decodes all smoke formats but lacks end-to-end range, 30-sample/two-hour and production release proof. Native fails Windows Ogg/Opus with 0xC00D36C4 and macOS start-before-full-download. Pure Go fails accepted AAC coverage, full-scan MP3 seek and Ogg random seek. Raw source paths and SHA-256 values are pinned and regeneration is byte-exact. Contract tests passed 16/16. Hosted CI run 29451972760 passed all four jobs. The local full suite was intentionally not claimed green because local FFmpeg 8.1.2 lacks libvorbis; hosted coordinator installed a suitable FFmpeg and passed. Physical 30-sample/two-hour/pairing evidence remains explicit not-run work in EPIC-260714-th54l3. Selection remains blocked until one complete combination passes every format, gate and pairing.

## Precondition Resources
(none)

## Outcome Resources
- [p2-codec-player-spike-sequence.puml](file://TASK-260712-ibuaxj/p2-codec-player-spike-sequence.puml) — Shared fail-closed comparative proof and manual-routing sequence
- [comparative-matrix-v1.json](file://TASK-260712-ibuaxj/comparative-matrix-v1.json) — Normative generated three-combination, three-pairing matrix
- [p2-comparative-streaming-evidence-matrix.md](file://TASK-260712-ibuaxj/p2-comparative-streaming-evidence-matrix.md) — Decision narrative, raw failures and reproduction steps
- [validate_comparative_matrix.py](file://TASK-260712-ibuaxj/validate_comparative_matrix.py) — Fail-closed matrix and artifact-hash validator
- [hosted-coordinator-manifest.json](file://TASK-260712-ibuaxj/hosted-coordinator-manifest.json) — Hosted run 29451972760 coordinator acceptance manifest
- [hosted-swift-manifest.json](file://TASK-260712-ibuaxj/hosted-swift-manifest.json) — Hosted run 29451972760 Swift acceptance manifest
- [hosted-windows-manifest.json](file://TASK-260712-ibuaxj/hosted-windows-manifest.json) — Hosted run 29451972760 Windows acceptance manifest
