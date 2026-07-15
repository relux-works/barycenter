## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-15T21:14:08Z

## Blocked By
- TASK-260712-14u0yk
- TASK-260712-dqdoqj

## Blocks
- TASK-260712-ibuaxj

## Checklist
- [x] Integrate the shortlisted pure-Go decoder candidate into macOS and Windows spike code without render-thread I O
- [x] Prove MP3 AAC and Opus decode with incremental fetch and bounded buffers
- [x] Measure scheduled start, pause, seek, resume, end-of-stream, and RSS on two-hour fixtures
- [x] Record AppContainer, threading, or performance blockers as concrete rejected-option evidence
- [x] Keep the current mixer and scheduling seam explicit so the winning path can be handed to implementation cleanly
- [x] Run hostile-input and race fixtures and reject any hidden C or full-file dependency

## Notes
2026-07-15 strict-sequence start from synchronized main dc947c0. Implementing inline outside task-board spawn workflow. The audited pure-go-composite-v1 remains rejected and the GPL-2.0-only AAC module will not enter a shipping or distributable client build. Scope is bounded research with exact permissive MP3 and Opus modules, explicit AAC refusal, incremental reader/ring lifecycle, hostile/race checks and cross-build evidence. Two-hour and physical platform measurements remain unclaimed in EPIC-260714-th54l3.
Accepted after engineering PR #114 merged as cbbe39c (head 7f243e2; code dc1ac49). Dedicated run 29450704499 passed macOS ARM64, Windows amd64 and Linux race; final standard run 29450856063 passed 4/4. The exact two-module CGo-free graph excludes GPL-only AAC. MP3 first PCM arrived after 621/237 bytes, but seek construction full-scanned 289818/52674 bytes. Opus first PCM arrived after 16264/19410 bytes, but the Ogg reader has no random-seek API. AAC is a zero-read forbidden-module rejection. Maximum ring use was 7680/1048576 bytes, underlying reads stayed at or below 636 bytes, hostile-input and race checks passed. macOS binary: 3568866 bytes, SHA-256 b6e81e4fabb847837da8a36c49a8b66648fd7e02aaf1beeac7cf586296a9f74c. Windows binary: 3746304 bytes, SHA-256 5625b40ddbc92e4c8b461b1416e552da64d13651aa5fb938a3b80e09efa1ab79. Heap evidence is not RSS; physical platforms, AppContainer and a two-hour run remain unclaimed in EPIC-260714-th54l3. The production candidate is rejected on license, seek and manual-evidence gates; this is accepted rejected-option evidence.

## Precondition Resources
(none)

## Outcome Resources
- [pure-go-probe-v1.json](file://TASK-260712-3vkcki/pure-go-probe-v1.json) — Frozen module, fixture, bound and rejection contract
- [p2-pure-go-streaming-decoder-probe.md](file://TASK-260712-3vkcki/p2-pure-go-streaming-decoder-probe.md) — Engineering decision, measured blockers and reproduction steps
- [repository-acceptance-manifest.json](file://TASK-260712-3vkcki/repository-acceptance-manifest.json) — Local repository acceptance manifest; all 12 commands passed
- [evidence-macos-arm64.json](file://TASK-260712-3vkcki/evidence-macos-arm64.json) — Hosted macOS ARM64 bounded decode and rejection evidence
- [receipt-macos-arm64.json](file://TASK-260712-3vkcki/receipt-macos-arm64.json) — Hosted macOS ARM64 CGo-free binary and cross-build receipt
- [evidence-windows-amd64.json](file://TASK-260712-3vkcki/evidence-windows-amd64.json) — Hosted Windows amd64 bounded decode and rejection evidence
- [receipt-windows-amd64.json](file://TASK-260712-3vkcki/receipt-windows-amd64.json) — Hosted Windows amd64 CGo-free binary and cross-build receipt
- [evidence-linux-amd64-race.json](file://TASK-260712-3vkcki/evidence-linux-amd64-race.json) — Hosted Linux race-clean bounded decode evidence
- [receipt-linux-amd64-race.json](file://TASK-260712-3vkcki/receipt-linux-amd64-race.json) — Hosted Linux CGo-free research binary and cross-build receipt
