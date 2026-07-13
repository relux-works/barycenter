# Add Telegram Air, N-target and track parity

## Description
Extend the secure Phase 1 Telegram adapter to Phase 2 selection and track commands without unique queue, rights or inbox logic.

## Scope
Map voice, audio and document ingest to the common media service; show versioned policy and rights prompts for applicable files; use opaque actor-bound callbacks for active-Air selection, explicit permitted Barycenter or node targets, include-origin, queue or replace and manual inbox replay or delete. Preserve immediate legacy voice after_current behavior. Use the common N-target and targeted-track policy, expose unsupported Phase 1 nodes and size, duration, rights or revocation errors, promptly answer callbacks and remove terminal keyboards.

## Acceptance Criteria
Telegram audio or document and app actions create identical media, queue or replace, target snapshot, consent, receipt, inbox and moderation semantics. Forged or stale callbacks cannot select foreign targets or replay hidden media. No personal-delivery broadcast fallback, raw ID, silent mixed-version behavior or transport-owned queue logic remains.
