# P2 Explicit targets, inbox and transport parity

## Description
Implement explicit target ACL snapshots, N-recipient personal delivery, offline inbox and Telegram parity.

## Scope
Replace both-or-single targeting with explicit N-recipient target snapshots and scoped media ACLs. Add missed-media inbox, TTL, manual replay/delete, receipt pagination and full app/Telegram parity for clips and tracks across Air rooms.

## Acceptance Criteria
B5-B7 pass. A non-target cannot fetch or discover media even with its ID. Personal delivery works for any member count without broadcast fallback. Offline and DND items appear in inbox but never late-autoplay. Telegram audio/document and app actions produce identical queue/replace/target semantics and rights/report enforcement.
