# P1 Generic media ingest and storage

## Description
Generalize Telegram voice processing into authenticated app/bot media ingest with validation, quotas and retention.

## Scope
Replace Telegram-specific intake ownership with a common SubmitMedia service and authenticated resumable upload sessions. Add media item persistence, signature and ffprobe validation, constrained ffmpeg normalization, idempotency, quotas, tenant-scoped dedupe, retention, delete and audit behavior while retaining legacy WAV compatibility.

## Acceptance Criteria
All phase-one input formats and limits in sections 7-8 are enforced by authoritative server probes. Interrupted/retried uploads do not duplicate media. Corrupt, oversized and timed-out inputs never become ready. Tenant ACL and deletion tests prevent cross-orbit access. Existing Telegram voice order and output remain compatible through the common service.
