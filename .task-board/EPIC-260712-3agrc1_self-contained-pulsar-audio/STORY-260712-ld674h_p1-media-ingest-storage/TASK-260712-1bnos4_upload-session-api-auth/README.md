# Add authenticated resumable media upload sessions

## Description
Add the HTTP upload-session surface for Pulsar clients using scoped upload credentials, resumable writes and quota enforcement without exposing long-lived control tokens in URLs.

## Scope
Implement POST and PUT upload-session endpoints, persistent progress, high-entropy short-lived scoped upload tokens stored or compared safely, declared-size and actual-byte limits, monotonic append offsets, idempotency keys, finalize state and phase-one rate, concurrent-processing and daily-byte quotas. Make concurrent or reordered writes deterministic, reject offset and length mismatch before processing, expire abandoned sessions and temporary bytes, keep long-lived control tokens and local paths out of URLs and logs, and expose only sanitized audit data.

## Acceptance Criteria
Interrupted uploads resume without duplicate media rows or orphan ready media. Unauthorized, expired-token, over-quota, oversize, offset-conflict, length-mismatch and repeated-finalize requests have stable non-disclosing results. One idempotency key maps to one result under concurrency; actual bytes cannot exceed declared or hard limits; cleanup and retry survive coordinator restart.
