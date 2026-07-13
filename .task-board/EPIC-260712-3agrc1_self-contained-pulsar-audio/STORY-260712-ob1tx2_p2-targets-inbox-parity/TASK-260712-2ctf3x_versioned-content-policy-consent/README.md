# Implement versioned content-policy acceptance

## Description
Record clear, localized UGC and file-rights consent before Phase 2 app-file or Telegram audio or document upload without pretending that consent replaces actual rights.

## Scope
Publish the current policy version and hash through the app and bot contract; record actor, orbit, version, locale and accepted_at using control or Telegram ActorContext; require a fresh accepted version before picked-file and Telegram audio or document upload as frozen by the contract; distinguish general Terms acceptance from per-upload rights reminder; re-prompt after material policy version change; allow policy display before acceptance; rate-limit and audit changes; and keep consent records free of content, filenames and raw transport IDs.

## Acceptance Criteria
No Phase 2 file upload begins without the required current acceptance and rights acknowledgement, while microphone and legacy paths follow the frozen policy rather than an accidental blanket block. Windows, macOS and Telegram show equivalent RU and EN text and exact policy links. Version change, revoke or actor mismatch has a deterministic result, and records prove acceptance without claiming ownership or legal validity beyond the approved policy.
