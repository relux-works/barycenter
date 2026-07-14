## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T19:20:08Z

## Blocked By
- TASK-260712-3coble

## Blocks
- TASK-260712-1c1ska
- TASK-260712-2hcq1g
- TASK-260712-21ers7
- TASK-260712-e1ie4x
- TASK-260712-2vipy3

## Checklist
- [x] Extract shared sender, target, audience and delivery label resolution out of ad hoc bot helpers.
- [x] Normalize receipt reason text and pairwise-approach naming across direct and linked deliveries.
- [x] Cover missing actor, slot and approach metadata with stable human fallbacks instead of raw identifiers.
- [x] Provide golden RU and EN labels for confirmation, downgrade and exact receipts
- [x] Prove no raw database, Telegram, node or composite peer identifier appears

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge b9e138f after TASK-260712-3coble acceptance. Scope is a shared transport-neutral RU/EN presentation model for sender/origin/audience/delivery/confirmation/downgrade/receipt semantics, with privacy-safe missing metadata fallbacks and golden leak guards. No manual or hardware evidence is claimed.
2026-07-14 implementation checkpoint: added coordinator/internal/presentation with transport-neutral key/en/ru labels for sender/origin/target/audience/include-origin, requested/effective delivery, downgrade, interrupt confirmation, every media/aggregate/target status and all 38 frozen receipt reasons. A SHA-256 RU/EN golden, exhaustive reason inventory, direct/linked fixtures and raw ID canaries are green. Legacy Telegram /home, /status, queue, voice target and provider errors now use the model; missing metadata never falls back to Telegram IDs or raw slots/composites. No manual or hardware evidence is claimed.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-1gx6mh/p1-telegram-history-presence-components.puml) — Component diagram for shared presentation-model ownership
- [p1-telegram-history-presence-states.puml](file://TASK-260712-1gx6mh/p1-telegram-history-presence-states.puml) — State diagram for shared receipt wording and lifecycle mapping
- [p1-shared-delivery-presentation-model.md](file://TASK-260712-1gx6mh/p1-shared-delivery-presentation-model.md) — Shared transport-neutral RU EN label model and privacy-safe fallback contract
