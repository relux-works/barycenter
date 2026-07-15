## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:21:21Z

## Last Update
2026-07-15T20:48:53Z

## Blocked By
- TASK-260712-14u0yk
- TASK-260712-dqdoqj

## Blocks
- TASK-260712-ibuaxj

## Checklist
- [x] Test native macOS MP3, AAC and Opus containers against every shared hard gate
- [x] Record sandbox, codesign, architecture and OS-version evidence

## Notes
2026-07-15 strict-sequence start from synchronized main c1842e2. Implementing inline outside task-board spawn workflow. Scope is a codesigned sandboxed native AVFoundation or AudioToolbox engineering probe over the frozen local range-cache boundary with exact smoke fixtures, lifecycle, timing, memory and fail-closed format evidence. Physical macOS hardware, audible output and supported-OS matrix remain explicitly unclaimed in manual-test epic EPIC-260714-th54l3.
Accepted after engineering PR #112 merged as bbd3f85 (engineering head fb4fe00; native code head c1ec5c0). The ad-hoc signed hardened-runtime app carries exactly com.apple.security.app-sandbox=true and no network entitlement, feeds AVAssetReader only through a 65,536-byte-bounded AVAssetResourceLoaderDelegate over sealed exact fixtures, and uses no audio output or render callback. Local repository acceptance passed 12/12. Dedicated run 29449314111 passed ARM64 and x86_64 jobs on macOS 15.7.7: all six MP3, AAC and Ogg/Opus fixtures decoded; seek-to-PCM stayed at or below 14 ms ARM64 and 21 ms Intel; peak RSS was 28,065,792 and 20,623,360 bytes. Both architectures read at least the complete source before first PCM, and cold MP3 CBR scheduled skew was 213 ms ARM64 and 466 ms Intel against the 100 ms gate. Candidate is therefore rejected for hidden full-file preparation and lifecycle timing, not selected for shipping. Exact executable/resource hashes and sandbox receipts are attached. Final standard CI run 29449425341 passed 4/4. Developer ID signing, notarization, physical hardware, audible routes and supported macOS matrix remain explicitly unclaimed in manual-test epic EPIC-260714-th54l3.

## Precondition Resources
- [p2-codec-player-spike-components.puml](file://TASK-260712-350u8d/p2-codec-player-spike-components.puml) — Codec candidate and streaming substrate boundaries
- [p2-codec-player-spike-sequence.puml](file://TASK-260712-350u8d/p2-codec-player-spike-sequence.puml) — Shared codec proof flow

## Outcome Resources
- [macos-native-probe-v1.json](file://TASK-260712-350u8d/macos-native-probe-v1.json) — Fail-closed sandbox, range, fixture and lifecycle contract
- [p2-macos-native-streaming-decoder-probe.md](file://TASK-260712-350u8d/p2-macos-native-streaming-decoder-probe.md) — Engineering decision, exact rejection evidence and reproduction steps
- [repository-acceptance-manifest.json](file://TASK-260712-350u8d/repository-acceptance-manifest.json) — Local repository acceptance manifest; all 12 commands passed
- [evidence-macos-arm64.json](file://TASK-260712-350u8d/evidence-macos-arm64.json) — Hosted ARM64 6/6 decode, range, lifecycle, timing and RSS evidence
- [receipt-macos-arm64.json](file://TASK-260712-350u8d/receipt-macos-arm64.json) — Hosted ARM64 signed app, sealed resource and entitlement receipt
- [evidence-macos-x86_64.json](file://TASK-260712-350u8d/evidence-macos-x86_64.json) — Hosted Intel 6/6 decode, range, lifecycle, timing and RSS evidence
- [receipt-macos-x86_64.json](file://TASK-260712-350u8d/receipt-macos-x86_64.json) — Hosted Intel signed app, sealed resource and entitlement receipt
