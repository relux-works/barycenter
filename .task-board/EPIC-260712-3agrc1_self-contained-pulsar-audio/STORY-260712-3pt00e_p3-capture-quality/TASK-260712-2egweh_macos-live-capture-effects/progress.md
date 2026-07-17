## Status
done

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
- TASK-260712-1getbv

## Checklist
- [x] Extend the macOS capture service for speaker, headphone and auto processing modes
- [x] Enforce AGC and local ceiling ordering plus typed degraded or unsupported states
- [x] Cover device, permission and lifecycle edges without stuck capture or stray streaming
- [x] Prove accepted speaker or headphone cases and degraded surfacing on real macOS hardware

## Notes
2026-07-17 strict sequential engineering start after TASK-260712-39czd2 merged. Implement best-effort macOS shared capture DSP and deterministic adapter coverage only; signed-app acoustic and physical hardware proof remains not-run in EPIC-260714-th54l3.
2026-07-17 accepted as best-effort engineering on exact head 1ccbb16a30ee1d6c5c1d60e479e911d8ea24b4af; PR #242 merged at a81e8fd3254ee342ba04594916e79298486e781b after hosted run 29567251374 passed 4/4. Clean exact-head swift suite passed 2/2 with 304 Swift tests and manualEvidence=not-run. Checklist item 4 is closed only as routed to manual EPIC-260714-th54l3; no physical speaker/headphone, acoustic C3, signed-app or listening result is claimed.

## Precondition Resources
- [p3-capture-quality-components.puml](file://TASK-260712-2egweh/p3-capture-quality-components.puml) — Task seam and dependency view for the macOS live capture implementation

## Outcome Resources
- [macos-capture-quality-engineering-v1.json](file://TASK-260712-2egweh/macos-capture-quality-engineering-v1.json) — Machine-readable best-effort engineering decision and honest manual boundary
- [p3-macos-capture-quality-implementation.md](file://TASK-260712-2egweh/p3-macos-capture-quality-implementation.md) — Selected macOS VPIO path, realtime boundary, route policy and limitations
- [clean-head-acceptance-manifest.json](file://TASK-260712-2egweh/clean-head-acceptance-manifest.json) — Clean exact-head Swift and acceptance-contract provenance for 1ccbb16
