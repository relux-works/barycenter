# Windows routing, presence, history and failure integration

## Description
Connect the Windows shell to the real Phase 1 onboarding, upload, transmission and playback-state services once they land.

## Scope
Bind Windows Create and Join, authenticated upload and transmission calls, routing for This Pulsar, own Barycenter and current approach, play-here, presence, history, receipts and allowed policy actions. Persist finalized unsent microphone or picked-file drafts across coordinator failure and process restart; implement idempotent resume or retry and explicit delete; remove local bytes only after confirmed upload. Self-test media remains separate and disposable. Render every protocol failure, requested versus effective delivery and confirmation honestly without hard-coding transport details.

## Acceptance Criteria
Windows completes A1 UI and the UI portions of A2, A6 and A7 without external credentials or false status. Outage and restart retain one retryable finalized draft rather than claiming sent or deleting it; retry cannot duplicate, explicit delete works, and upload confirmation cleans local bytes. Self-test never enters network history. Receipt, presence, routing and policy labels match canonical models without raw IDs.
