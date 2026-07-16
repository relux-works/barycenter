## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:36:45Z

## Last Update
2026-07-16T18:21:08Z

## Blocked By
- TASK-260712-3qviqc
- TASK-260712-30abcm

## Blocks
- TASK-260712-2kj9kj

## Checklist
- [x] Bind only a validated local hold generation to capture
- [x] Encode off callbacks into a hard-bounded send queue
- [x] Prove watchdog release sleep revoke disconnect and backpressure cleanup
- [x] Prove no media persistence sample logging or remote capture start
- [x] Run supported macOS sender stress evidence

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge b0a63791da4de48708492bf7a32f64604291abde. Execute inline outside task-board spawn. Physical hold UX, real microphone/device/sleep behavior, audible cues and 100-cycle hardware evidence remain manual in TASK-260712-1rzqh9; this task proves best-effort bounded capture/encode/send code, deterministic lifecycle stress and callback safety.
2026-07-16 accepted. Code d5868f9 landed through PR 193 at merge eac1c183144df93ea126c9c595bb6dca8a8cd842 after local clean acceptance 12/12, Swift 273/273, focused sender 7/7 including 100 deterministic cycles, and hosted CI 29523191600 passed 4/4. Generation-safe local-only start, fixed mailbox, off-callback Apple raw Opus engineering encode, eight-frame queue and idempotent lifecycle teardown are implemented. live_ptt_v1 remains unadvertised because Apple exposes no exact libopus FEC or complexity controls. Physical hold, microphone, device, sleep, lock, audible-cue and hardware-cycle evidence remains manual in TASK-260712-1rzqh9.

## Precondition Resources
(none)

## Outcome Resources
- [p3-macos-live-capture-sender.md](file://TASK-260712-26mnp1/p3-macos-live-capture-sender.md) — Accepted macOS bounded sender implementation, deterministic evidence, production boundary and rollback handoff
