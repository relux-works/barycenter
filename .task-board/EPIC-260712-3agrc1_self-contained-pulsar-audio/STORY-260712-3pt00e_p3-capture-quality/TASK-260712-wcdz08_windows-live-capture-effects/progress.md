## Status
development

## Assigned To
codex-inline-engineer

## Created
2026-07-12T16:25:28Z

## Last Update
2026-07-17T08:46:24Z

## Blocked By
- TASK-260712-1gmsvh
- STORY-260712-2e36uz
- STORY-260712-sskhip
- STORY-260712-fes2jj
- TASK-260712-39czd2

## Blocks
- TASK-260712-39zh8g

## Checklist
- [ ] Extend the Windows capture service for speaker, headphone and auto processing modes
- [ ] Enforce AGC and local ceiling ordering plus typed degraded or unsupported states
- [ ] Cover device, permission and lifecycle edges without stuck capture or stray streaming
- [ ] Prove accepted speaker or headphone cases and degraded surfacing on signed Windows hardware

## Notes
2026-07-17 strict sequential engineering start after TASK-260712-2egweh merged. Implement best-effort Windows shared capture DSP and deterministic coverage only; signed-app acoustic and physical hardware proof remains not-run in EPIC-260714-th54l3.

## Precondition Resources
- [p3-capture-quality-components.puml](file://TASK-260712-wcdz08/p3-capture-quality-components.puml) — Task seam and dependency view for the Windows live capture implementation

## Outcome Resources
(none)
