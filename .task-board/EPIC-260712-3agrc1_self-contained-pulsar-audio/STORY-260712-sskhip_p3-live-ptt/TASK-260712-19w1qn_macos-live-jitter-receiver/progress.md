## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:36:46Z

## Last Update
2026-07-16T17:55:40Z

## Blocked By
- TASK-260712-3qviqc
- TASK-260712-1hqiek
- TASK-260712-2zbmq4

## Blocks
- TASK-260712-2kj9kj

## Checklist
- [x] Validate session generation sequence profile and authorization before decode
- [x] Decode off render into a bounded jitter and PCM path
- [x] Exercise FEC or PLC late-drop loss jitter and stale-frame cases
- [x] Integrate pre-duck release ceiling limiter and click-free terminal cleanup
- [x] Prove AVAudio render has no waits allocation disk or network work

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge 1b2157a8997335fc6c36001686343fd04daa6985. Execute inline outside task-board spawn. Physical intelligibility, audible recovery, latency and real-hardware evidence remain routed to TASK-260712-1rzqh9; this task proves best-effort bounded code, deterministic unit/integration behavior and render-thread safety.
2026-07-16 accepted via PR #191, merge 9c1d0c2a4e3fc2bb0f339ccef57945ac5ffa4f4c, hosted CI 29521325367 4/4 and clean automated acceptance 12/12. Added a nine-packet bounded jitter window, 320 ms PCM ring, system raw-Opus decode, explicit injected-FEC or bounded PLC recovery, stale/malformed rejection, generation-safe drain/cancel/timeout cleanup and a render-only source branch through the shared limiter. Swift suite 263/263 and focused receiver 8/8 passed. Physical intelligibility, audible PLC/click quality, calibrated latency and real-hardware evidence remain manual in TASK-260712-1rzqh9 under EPIC-260714-th54l3; live_ptt_v1 stays unadvertised and production-disabled.

## Precondition Resources
(none)

## Outcome Resources
- [TASK-260712-19w1qn_macos-live-jitter-receiver.md](file://TASK-260712-19w1qn/TASK-260712-19w1qn_macos-live-jitter-receiver.md) — Accepted macOS jitter receiver bounds, loss recovery, render safety and manual evidence boundary
