## Status
development

## Assigned To
codex-inline-engineer

## Created
2026-07-12T16:25:28Z

## Last Update
2026-07-17T06:54:05Z

## Blocked By
- TASK-260712-1gmsvh

## Blocks
- TASK-260712-39zh8g
- TASK-260712-1getbv

## Checklist
- [ ] Extend shared state or protocol surfaces and keep codec mirrors in sync
- [ ] Surface route mode, effect status and input-health guidance in Windows and macOS shells
- [ ] Add sanitized counters or logs for degradation, clipping, low-input and device loss
- [ ] Verify mixed-version and unsupported-target wording stays honest on both platforms

## Notes
2026-07-17 strict sequential engineering start queued after capture-quality contract merged. Manual Windows/macOS platform probes remain in EPIC-260714-th54l3 and are skipped in the engineering lane. Implement exact additive protocol/heartbeat/diagnostics mirrors from capture-quality.v1 without advertising production capability or inventing hardware parity.

## Precondition Resources
- [p3-capture-quality-components.puml](file://TASK-260712-1pw1l1/p3-capture-quality-components.puml) — Task seam and dependency view for diagnostics and capability surfaces
- [p3-capture-quality-validation.puml](file://TASK-260712-1pw1l1/p3-capture-quality-validation.puml) — Live capture and degraded-surface flow for shared diagnostics work

## Outcome Resources
(none)
