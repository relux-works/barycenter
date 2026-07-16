## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:25:04Z

## Last Update
2026-07-16T20:01:47Z

## Blocked By
- TASK-260712-26mnp1
- TASK-260712-19w1qn
- TASK-260712-3vzbbl

## Blocks
- TASK-260712-1rzqh9
- TASK-260712-3980vy

## Checklist
- [ ] Hook the supported global key-down or key-up path into the existing macOS capture and menu lifecycle
- [x] Stream encoded live chunks with session generations, cancel semantics and backpressure handling
- [x] Add a bounded receiver jitter buffer and live duck or un-duck integration in the audio graph
- [x] Stop capture and playback safely on release, lock or sleep where observable, quit, reconnect or stale session
- [x] Add platform tests or probes and publish remaining hardware-only evidence needs

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge 3f7d7643e1ae9b59d7f9b2301acb2234af7713f3. Execute inline outside task-board spawn. Integrate accepted macOS live sender/receiver seams production-dark; do not advertise live_ptt_v1 or claim real app/audio/hardware evidence, which remains manual in TASK-260712-1rzqh9.
2026-07-16 engineering completion: production-dark macOS live PTT node integrates authoritative target/policy seams, generation-bound sender, bounded binary websocket transport, jitter receiver/mixer status and lifecycle cleanup. Code e7472b2146b60b850984a6dff91cab31b1ca0b6b; PR #199; hosted run 29529995520 passed 4/4; main merge f33f1fbb8330ce946e5ecf748f7a522d2ba32d81. Focused Swift 24/24, full Swift 280/280, clean automated acceptance 12/12 at .temp/acceptance/20260716T195339Z/manifest.json. TSan-instrumented build completed but runtime was unavailable because macOS rejected the Xcode sanitizer dylib signature. Checklist item 1 remains intentionally unchecked: no production global key-down/up hook or Accessibility request was added; real-app hold, audio and hardware evidence stays manual in TASK-260712-1rzqh9 under EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
- [p3-live-ptt-components.puml](file://TASK-260712-2kj9kj/p3-live-ptt-components.puml) — Task-boundary diagram for macOS live PTT integration
- [p3-live-ptt-sequence.puml](file://TASK-260712-2kj9kj/p3-live-ptt-sequence.puml) — Live session sequence that the macOS node must implement
- [p3-macos-live-ptt-node-integration.md](file://TASK-260712-2kj9kj/p3-macos-live-ptt-node-integration.md) — Engineering integration boundary, deterministic evidence and manual activation blockers
