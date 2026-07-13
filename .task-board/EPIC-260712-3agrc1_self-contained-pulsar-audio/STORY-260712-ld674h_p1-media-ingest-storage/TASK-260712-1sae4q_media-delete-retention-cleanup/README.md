# Implement media delete, retention and physical cleanup

## Description
Implement the independently testable media lifecycle after ingest, including immediate logical deletion and retry-safe byte cleanup.

## Scope
Implement owner or permitted-control DELETE authorization; atomically mark media deleted, revoke new downloads and request cancellation of every pending transmission; apply the frozen sender-delete policy to prepared, scheduled and already-playing targets; enqueue physical byte removal without blocking the request; sweep failed uploads within 24 hours, ready clips at the seven-day default, expired upload sessions and already-deleted storage; retain transmission, report and audit metadata only according to their separate policies; make cleanup idempotent across crashes and document backup-retention limits in the privacy handoff.

## Acceptance Criteria
Deleting owned media immediately prevents new download and late autoplay, changes pending or active targets to the frozen exact deleted outcome, and eventually removes canonical and temporary bytes. Unauthorized deletion is uniformly non-disclosing. Repeated or interrupted sweeps are safe, do not delete live media, satisfy phase-one retention defaults, expose operator metrics and preserve only policy-required audit metadata.
