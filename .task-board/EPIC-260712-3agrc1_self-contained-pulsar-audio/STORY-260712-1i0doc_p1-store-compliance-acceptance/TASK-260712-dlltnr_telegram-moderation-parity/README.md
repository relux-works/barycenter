# Add secure Telegram and shared-history moderation parity

## Description
Render the canonical replay, report, block and delete actions in Telegram and shared history without creating a second moderation implementation.

## Scope
Consume the actor-bound callback transport, history action capabilities and canonical moderation command service. Expose only allowed Report, mute or block and owner Delete actions; use integrity-protected expiring callbacks, frozen reason and status labels, prompt acknowledgements and terminal keyboard removal. Preserve immediate legacy after_current behavior, target receipts and callback race rules. Add app-facing shared-history mapping but leave Windows and macOS rendering to their tasks.

## Acceptance Criteria
Telegram and app history invoke the same authorized actions and receive the same privacy-safe statuses. Forged, expired, cross-user, repeated and group callbacks cannot act. Legacy voice ordering and inline delivery remain unchanged, raw IDs never appear, and RU or EN semantic labels match the app model.
