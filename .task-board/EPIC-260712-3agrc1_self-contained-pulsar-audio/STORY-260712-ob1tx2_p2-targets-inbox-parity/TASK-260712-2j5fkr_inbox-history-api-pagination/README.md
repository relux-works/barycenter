# Expose actor-scoped inbox, history, receipts and replay APIs

## Description
Provide one non-disclosing paginated read and command surface over the extended Phase 1 models.

## Scope
Implement authenticated inbox list and detail, history and receipt pagination, allowed-action projection, manual local or targeted replay, dismiss, sender delete and eligible cancel commands. Use stable opaque cursors and deterministic ordering under concurrent inserts, ActorContext and accepted-target authorization, idempotency for commands, expiry and deleted-state checks, rate limits and uniform missing behavior. Never return raw media, transmission, actor, orbit or node IDs where an opaque reference or localized label suffices, and never trigger autoplay from list or reconnect.

## Acceptance Criteria
Authorized actors see only their permitted items and receipts with stable pagination and accurate partial or unsupported state. Non-target, revoked and newly joined actors cannot infer an item by ID or cursor. Replay is always explicit, creates one new delivery or local playback, and fails honestly after expiry, delete or disable. Repeated or racing commands are idempotent and no inbox read changes playback.
