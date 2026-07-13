# Extend canonical rights and moderation enforcement to tracks and inbox

## Description
Apply Phase 1 report, block, delete and disable services plus versioned consent to range fetch, track queues and missed-item replay.

## Scope
Consume the canonical moderation control plane, media lifecycle, actor revocation, block service and active-delete policy. Gate frozen Phase 2 file-upload paths on current consent. Report creation must not become a global denial-of-service primitive: apply only reporter-local hide or block and any explicitly reviewed quarantine state; moderator or sender delete, actor disable and orbit disable prevent future range or chunk fetch, queue start and inbox replay and terminally mark affected inbox items. Audit sanitized outcomes and preserve already-started behavior from the Phase 1 delete contract. Do not add parallel report or block persistence.

## Acceptance Criteria
B7 passes through the same app and Telegram workflow: missing consent blocks applicable upload, every accessible foreign item is reportable, the reporter is protected immediately, and an authorized delete or disable decision prevents direct URL, range, cache-refill, queue and inbox access. A malicious report cannot globally censor content by itself. Existing blocks, reports and audits remain the source of truth and no transport special case bypasses enforcement.
