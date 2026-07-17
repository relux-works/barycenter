## Status
done

## Assigned To
codex-inline-engineer

## Created
2026-07-12T16:42:28Z

## Last Update
2026-07-17T09:56:48Z

## Blocked By
- TASK-260712-2egweh
- TASK-260712-1pw1l1
- TASK-260712-3dqc3l

## Blocks
- TASK-260712-1023d7

## Checklist
- [x] Render route mode processor state and both distinct ceilings
- [x] Warn before degraded speaker capture and offer the approved fallback
- [x] Keep active capture and local Stop visible in window and menu bar
- [x] Cover permission device reference route and mixed-version failures
- [x] Run VoiceOver keyboard lifecycle and no-remote-start checks

## Notes
2026-07-17 strict sequential engineering start after TASK-260712-wcdz08 merged; execute inline outside task-board spawn workflow. Real-app visual, accessibility and hardware evidence remains manual in EPIC-260714-th54l3.
2026-07-17 accepted best-effort UI engineering on exact code head c589c59ce252f12d4c50e453a5bd1d260d13e6a9; clean acceptance passed 142 contract and 307 Swift tests with manualEvidence=not-run, release build passed, hosted run 29571493442 passed 4/4, and PR #246 merged as 8706dbd88d11d02f59a2bf5a6878ca64524fc8e1. Checklist item 5 closes source/unit keyboard, accessibility-semantics, lifecycle and no-remote-start coverage only; real signed-app VoiceOver, focus order, visual, TCC, physical route/acoustic and stop-latency checks remain explicitly not-run in EPIC-260714-th54l3. Production clip and self-test are wired; live_ptt presentation is modeled but the production MacLivePTTNode remains dark and no live UI pass is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p3-macos-capture-quality-ui.md](file://TASK-260712-1getbv/p3-macos-capture-quality-ui.md) — Honest macOS capture-quality UI implementation and manual boundary
- [macos-capture-quality-ui-engineering-v1.json](file://TASK-260712-1getbv/macos-capture-quality-ui-engineering-v1.json) — Machine-readable UI engineering decision and not-run evidence boundary
- [task-260712-1getbv-clean-c589c59-manifest.json](file://TASK-260712-1getbv/task-260712-1getbv-clean-c589c59-manifest.json) — Clean exact-head acceptance manifest: 142 contract and 307 Swift tests
