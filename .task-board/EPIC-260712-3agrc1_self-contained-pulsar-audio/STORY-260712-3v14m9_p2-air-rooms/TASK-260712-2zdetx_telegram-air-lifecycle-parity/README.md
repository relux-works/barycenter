# Add secure Telegram Air lifecycle parity

## Description
Expose Air 2-to-N management through the Phase 1 actor-bound Telegram adapter without bot-specific lifecycle logic.

## Scope
Use common Air services and opaque expiring callbacks to list saved or current Airs, create, issue and consume invites, obtain the joining-primary confirmation, decline or withdraw, activate or switch, leave, dissolve when permitted and view or change allowed policies. Bind every callback to ActorContext and role, answer promptly, remove terminal keyboards, use human RU and EN room and Barycenter labels, preserve immediate approach or apart aliases and never expose invite secrets or raw IDs in logs or callback text.

## Acceptance Criteria
Telegram and Pulsar perform identical authorized Air transitions, policy changes and errors. A group member cannot confirm or switch another Barycenter without role, forged, stale or repeated callbacks cannot act, invites remain single-use and redacted, and legacy approach or apart wording continues through the same service with no duplicate runtime.
