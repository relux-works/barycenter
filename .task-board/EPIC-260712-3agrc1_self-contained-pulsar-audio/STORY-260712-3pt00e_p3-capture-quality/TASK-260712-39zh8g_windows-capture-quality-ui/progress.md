## Status
done

## Assigned To
codex-inline-engineer

## Created
2026-07-12T16:42:28Z

## Last Update
2026-07-17T10:29:20Z

## Blocked By
- TASK-260712-wcdz08
- TASK-260712-1pw1l1
- TASK-260712-2fe5bz

## Blocks
- TASK-260712-1023d7

## Checklist
- [x] Render route mode processor state and both distinct ceilings
- [x] Warn before degraded speaker capture and offer the approved fallback
- [x] Keep active capture and local Stop visible in window and tray
- [x] Cover permission device reference route and mixed-version failures
- [x] Run signed accessibility DPI lifecycle and no-remote-start checks

## Notes
2026-07-17 strict sequential engineering start after TASK-260712-1getbv merged; execute inline outside task-board spawn workflow. Real signed-app accessibility, DPI, visual, physical route and acoustic evidence remains manual in EPIC-260714-th54l3.
2026-07-17 accepted best-effort UI engineering on exact code head de40dcb71387e9e3e422e72adf6f999cb0572212; clean Windows acceptance passed 11/11 with start/end dirty false and manualEvidence=not-run, hosted run 29573283583 passed 4/4, and PR #248 merged as def2aa2845175efac4a06f942cf2468e5b8e6ca7. Checklist item 5 closes source/unit native-control, keyboard, DPI-aware layout, reconnect/lifecycle and no-remote-start semantics only; real signed-MSIX Narrator/focus, DPI visual, permission, physical route/reconnect, acoustic and stop-latency checks remain explicitly not-run in EPIC-260714-th54l3. Production clip and self-test are wired; live_ptt presentation is modeled but shipping main still does not construct WindowsLivePTTNode or advertise the capability.

## Precondition Resources
(none)

## Outcome Resources
- [p3-windows-capture-quality-ui.md](file://TASK-260712-39zh8g/p3-windows-capture-quality-ui.md) — Honest Windows capture-quality UI implementation and manual boundary
- [windows-capture-quality-ui-engineering-v1.json](file://TASK-260712-39zh8g/windows-capture-quality-ui-engineering-v1.json) — Machine-readable UI engineering decision and not-run evidence boundary
- [task-260712-39zh8g-clean-de40dcb-manifest.json](file://TASK-260712-39zh8g/task-260712-39zh8g-clean-de40dcb-manifest.json) — Clean exact-head Windows acceptance manifest: 11 of 11 stages
