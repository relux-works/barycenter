# Root review round 3 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: nearly complete, but four implementation-blocking contradictions remain.

## Blocking corrections

1. **Preserve legacy node-token hashes.**
   Existing `slots.token_hash` values are unkeyed SHA-256 and their plaintext
   tokens are intentionally unavailable. They cannot be converted to HMAC.
   Likewise, the proposed “offline re-hash with a new HMAC key” is
   cryptographically impossible without plaintext. Use the existing exact
   SHA-256 contract for high-entropy node/control/recovery/link material, or
   define a versioned dual-scheme resolver and migration that leaves every
   legacy node hash valid. The minimal approved choice is unkeyed SHA-256 for
   all of these uniformly random secrets; remove the HMAC key file, impossible
   rotation claim, and related operational failure modes. Server still stores
   hashes only and compares fixed 32-byte digests in constant time.

2. **Never delete an uncertain pending token.**
   A generic `403` after restart can mean the user mistyped the recovery secret
   even though the server already committed the pending replacement token.
   Deleting it would destroy the sole working control credential. Add a
   read-only authenticated actor-context probe for the pending candidate (exact
   endpoint/response) and freeze this order on startup/network uncertainty:
   candidate auth probe → if valid, promote; if invalid, ask for the recovery
   secret and retry the same tuple. A `403` from recovery alone is never proof
   that the candidate is inactive. Explicit Cancel may hide the flow but must
   retain protected pending state until candidate invalidity is proven, or give
   a very explicit destructive-abandon confirmation. Update client tasks/tests.

3. **Make schema ownership internally consistent.**
   The source model places installation secrets in an installation-credential
   entity, but rev 3 puts node/control hashes directly on `actors` while also
   referring to multiple old control-token records. Freeze one additive model:
   actors/memberships remain identity and role; installation credentials own
   current node/control/recovery state (or reference the authoritative legacy
   slot hash); existing `slots` and their token hashes remain valid and do not
   drift through duplicated mutable copies. State whether replacing one current
   control hash is the revocation mechanism or whether a version/history table
   exists. Downstream schema code must not invent this.

4. **Make authorization errors and attempt limits consistent.**
   The note reserves `credential_invalid` for unauthenticated secret consumes,
   but the authenticated Telegram-issuance matrix uses it for disabled orbits.
   Use `401` for invalid/revoked bearer and `403 insufficient_capability` for a
   valid token lacking active role/orbit authority. Also make consume limits
   concurrency-safe: atomically reserve/count a syntactically valid attempt
   before hash work, otherwise many parallel requests can all pass a “failed
   count” checked before any failure is recorded. It is acceptable and simpler
   to count attempts, not only failures; freeze exact semantics for recovery,
   device invite, and Telegram-code consume.

## Resubmission

- Amend the one authoritative note/outcome; no source-code edits.
- Preserve the accepted integer IDs, 128-bit recovery handle, unbiased
  27-character codes, retry-idempotent server transaction, in-process Telegram
  principal, exact error envelope, HTTPS/no-store/redaction, and role matrix.
- Update all examples, downstream impacts, and tests to the same decisions.
- Return to `to-review`, never `done`.
