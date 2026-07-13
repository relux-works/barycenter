# Prove Telegram, history and presence parity and security

## Description
Add deterministic tests for legacy behavior, callback authorization, read-model isolation, recipient controls and localized presentation.

## Scope
Cover legacy default enqueue timing and ordering when processing completes out of order; atomic callback replacement versus playback start; duplicate, forged, expired, cross-user and group callback events; interrupt confirmation; voice, audio and document errors; history pagination, tenant isolation and action authorization; exact receipts; DND precedence and expiry; block roles; presence staleness and sanitization; pairwise naming and RU or EN parity. Include mixed new and legacy nodes.

## Acceptance Criteria
Tests prove no callback can hijack another actor or duplicate playback, no history or presence query leaks tenant, microphone, process, device or raw identifier data, and legacy no-action voice remains first after current with no new wait. DND, block, downgrade and confirmation reasons are exact, action permissions are enforced and app versus bot semantic labels match in both locales.
