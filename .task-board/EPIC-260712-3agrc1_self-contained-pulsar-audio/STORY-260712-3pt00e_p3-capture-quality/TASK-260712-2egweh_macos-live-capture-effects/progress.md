## Status
development

## Assigned To
codex-inline-engineer

## Created
2026-07-12T16:25:28Z

## Last Update
2026-07-17T08:08:25Z

## Blocked By
- TASK-260712-1gmsvh
- STORY-260712-2e36uz
- STORY-260712-sskhip
- STORY-260712-fes2jj
- TASK-260712-39czd2

## Blocks
- TASK-260712-1getbv

## Checklist
- [ ] Extend the macOS capture service for speaker, headphone and auto processing modes
- [ ] Enforce AGC and local ceiling ordering plus typed degraded or unsupported states
- [ ] Cover device, permission and lifecycle edges without stuck capture or stray streaming
- [ ] Prove accepted speaker or headphone cases and degraded surfacing on real macOS hardware

## Notes
2026-07-17 strict sequential engineering start after TASK-260712-39czd2 merged. Implement best-effort macOS shared capture DSP and deterministic adapter coverage only; signed-app acoustic and physical hardware proof remains not-run in EPIC-260714-th54l3.

## Precondition Resources
- [p3-capture-quality-components.puml](file://TASK-260712-2egweh/p3-capture-quality-components.puml) — Task seam and dependency view for the macOS live capture implementation

## Outcome Resources
(none)
