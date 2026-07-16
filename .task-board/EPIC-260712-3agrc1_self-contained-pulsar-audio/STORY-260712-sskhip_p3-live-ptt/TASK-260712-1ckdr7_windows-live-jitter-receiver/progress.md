## Status
done

## Assigned To
(none)

## Created
2026-07-12T16:36:45Z

## Last Update
2026-07-16T18:56:52Z

## Blocked By
- TASK-260712-3qviqc
- TASK-260712-1hqiek
- TASK-260712-1viwvi

## Blocks
- TASK-260712-2jbo5i

## Checklist
- [x] Validate session generation sequence profile and authorization before decode
- [x] Decode off render into a bounded jitter and PCM path
- [x] Exercise FEC or PLC late-drop loss jitter and stale-frame cases
- [x] Integrate pre-duck release ceiling limiter and click-free terminal cleanup
- [x] Prove WASAPI render has no waits allocation disk or network work

## Notes
2026-07-16 strict-sequence engineering start from synchronized main merge 2adbe57f4a0591a08a3297e5db6cf2dbf3486c9a. Execute inline outside task-board spawn. Real Windows audio, signed packaged-app playback, two-home loss, intelligibility, latency and hardware cleanup evidence remains manual in TASK-260712-1rzqh9; this task proves best-effort bounded jitter, decoder, render and deterministic lifecycle code.
2026-07-16 accepted. Code 987de8b, PR #195, merge 365fb117e04d2bb8f462b7cd3bd29b7339d797a5. Hosted CI run 29525402024: 4/4 passed. Local go vet/test/race, focused Windows live receiver suite, render-safety checks, amd64/arm64 Windows cross-builds and clean 12/12 acceptance manifest .temp/acceptance/20260716T184502Z/manifest.json passed. Production decoder/capability intentionally remains unadvertised under accepted codec no-go; real signed Windows hardware playback, intelligibility, loss/latency and cleanup evidence is owned by manual TASK-260712-1rzqh9.

## Precondition Resources
(none)

## Outcome Resources
- [windows-live-jitter-receiver-evidence](file://TASK-260712-1ckdr7/windows-live-jitter-receiver-evidence) — Best-effort code, unit/race/cross-build and hosted acceptance evidence; physical Windows audio remains manual
