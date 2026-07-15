## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:45:04Z

## Last Update
2026-07-15T07:44:11Z

## Blocked By
- TASK-260712-1epb3a
- TASK-260712-g9ycx5
- TASK-260712-6kba80
- TASK-260712-1gx6mh

## Blocks
- TASK-260712-2s4e9p
- TASK-260712-1xik11

## Checklist
- [x] Audit the Windows manifest, macOS bundle metadata and shared localized strings for Spotify first or Telegram required language.
- [x] Add microphone declaration and usage text consistent with explicit record only behavior.
- [x] Define canonical RU and EN terminology for self contained onboarding, routing, receipts and optional integrations.
- [x] Record which downstream UI, listing and certification artifacts must consume the approved copy.
- [x] Prove explicit-Record permission timing and useful denial behavior
- [x] Run manifest schema checks and WACK with no broad filesystem or silent Accessibility capability

## Notes
2026-07-15 strict inline kickoff from synchronized main merge 4bc8418 after accepted Telegram moderation parity. Ivan Oparin approved the supplied legal operations and product defaults. Engineering scope is minimal truthful manifests localized copy static schema validation explicit-action permission wiring and automated denial-path coverage. Actual WACK UI real packaged-app permission prompts physical hardware and screenshots remain manual in EPIC-260714-th54l3 and are not claimed here.
2026-07-15 engineering candidate: canonical EN/RU platform-copy contract is bound to both desktop shells, localized MSIX .resw/resources.pri staging and localized macOS privacy strings; manifests keep the exact reviewed capability set and add no Accessibility or broad filesystem entitlement. Legacy Spotify and Telegram flows are plainly optional and Spotify help is no longer auto-presented. Local full make test, Windows Go suite, XML/plist/YAML validation and an ad-hoc macOS bundle assembly passed. Checklist item 6 records completed static schema/package-equivalent validation only; actual WACK UI, signed installed prompts and physical hardware remain explicitly deferred to EPIC-260714-th54l3 / TASK-260712-1vtwkl and are not claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-platform-declarations-localized-copy.md](file://TASK-260712-e1ie4x/p1-platform-declarations-localized-copy.md) — Reviewed declarations, canonical copy, packaging consumers, and manual evidence boundary
