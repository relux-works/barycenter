## Status
to-review

## Assigned To
(none)

## Created
2026-07-12T15:45:04Z

## Last Update
2026-07-15T10:30:45Z

## Blocked By
- TASK-260712-1epb3a
- TASK-260712-pbfz37
- TASK-260712-34stvx
- TASK-260712-dlltnr
- TASK-260712-3t9nr8
- TASK-260712-e1ie4x
- TASK-260712-g9ycx5
- TASK-260712-16zfvu
- TASK-260712-1x0lot

## Blocks
- (none)

## Checklist
- [x] Replace the current Spotify first listing copy with self contained phase one descriptions and feature bullets in RU and EN.
- [x] Prepare the IARC and certification note answer set with evidence links for microphone, UGC, reporting and reviewer testability.
- [ ] Define and collect the real screenshot set required per language and per Microsoft feedback history.
- [ ] Save the exact Partner Center submission inputs and versioned source files used for the asset pack.
- [ ] Collect real localized primary-function screenshots at current official dimensions
- [x] Directly answer Product ID 9P26FDCWV1GC findings 10.3.1 and 10.1.1.3
- [ ] Run WACK and version every exact Partner Center input against the submitted build

## Notes
2026-07-15 strict engineering started from synchronized main 6664ffd after security tracking PR #75 passed hosted run 29405188885. Scope is versioned EN/RU listing, IARC/certification answer source, validators and exact-build asset manifest. Real localized screenshots, WACK execution and Partner Center mutation remain manual/external evidence and will not be self-claimed.
Engineering package now validates exact EN/RU copy, approved links, feature/keyword limits, optional Spotify/Telegram wording, Phase 1 limitations, IARC truth facts, certification findings and six screenshot slots per locale. Default validation passes; --require-ready fails closed on absent real evidence. Live policy pages match approved hashes. Manual screenshots/WACK are routed to existing TASK-260712-e5mfqj without changing the 205-task inventory; Partner Center/IARC/exact-build owner completion is TASK-260715-24ube9. Checklist items that require those observations remain open.
2026-07-15 exact engineering head 99f1957 passed clean acceptance 12/12; hosted run 29406679102 passed all four jobs. PR #76 merged at ee0cf03. Manual completion remains in TASK-260712-e5mfqj and Partner Center/IARC/exact-build owner completion in TASK-260715-24ube9. Original Store asset task remains to-review and is not counted accepted; strict engineering advances to TASK-260712-38lssj.

## Precondition Resources
(none)

## Outcome Resources
- [phase1-partner-center-package.md](file://TASK-260712-2s4e9p/phase1-partner-center-package.md) — Versioned EN/RU listing, IARC truth profile, certification notes, screenshot/WACK gates and completion procedure
- [partner-center-package.json](file://TASK-260712-2s4e9p/partner-center-package.json) — Machine-validated Partner Center package authority
