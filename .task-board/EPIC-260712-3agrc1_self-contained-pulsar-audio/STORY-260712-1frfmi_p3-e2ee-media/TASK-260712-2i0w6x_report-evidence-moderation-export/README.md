# Implement explicit decrypted report-evidence export

## Description
Let a recipient deliberately create a moderation copy while making the boundary crossing visible and tightly controlled.

## Scope
Offer metadata-only report by default and a second explicit consent action to decrypt selected media locally, package the exact media ID, sender signature or authenticated manifest hash and reporter statement, and upload a purpose-limited evidence copy. Mark that copy as no longer E2EE, encrypt it at rest for the moderation service, enforce least-privilege operator access, short configured TTL, immutable audit and canonical delete or disable actions. Prevent background export, coordinator-side decryption, further history export and misleading claims that reporter-provided evidence is independently authentic.

## Acceptance Criteria
No decrypted evidence exists server-side without a specific recipient action and consent receipt. Authorized moderation can access only the supplied scoped copy and allowed metadata; every create, read and delete is audited and expires. Metadata-only reporting remains functional, revoked access blocks new exports and tests prove storage or traffic capture before consent contains no plaintext.
