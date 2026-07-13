# Root review round 2 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: changes still required. R1 fixed the broad design, but the revised
contract is not yet safe to hand to schema/API/client developers.

## Blocking corrections

1. **Close the client crash gap, not only the lost-response gap.**
   The document still says only “generate, then send”. The client MUST write the
   256-bit replacement control token to Keychain/DPAPI as a protected *pending*
   credential before sending the destructive consume request. Freeze promotion,
   retry, authenticated verification, and discard behavior. A process/machine
   crash after server commit must leave enough protected state to recover using
   the same tuple; the recovery secret itself remains user-supplied and is not
   silently saved.

2. **Do not invent a second orbit-ID system.**
   The live schema and APIs use `orbits.id INTEGER`/Go `int64`. The note's
   `orb_` random strings have no additive mapping, backfill, rollback, or legacy
   compatibility story. Freeze `orbit_id` as the existing JSON integer. Use an
   additive SQLite-compatible actor key (prefer INTEGER primary key) and state
   its exact JSON type. If a separate public ID is truly required, it needs an
   explicit mapped column, uniqueness/collision retry, backfill, and dual-read
   contract; that expansion is not currently justified. Recovery response for
   Phase 1 should have one unambiguous membership shape, not both a scalar role
   and a speculative multi-membership array.

3. **Increase the stable recovery handle to 128 random bits.**
   `rec_` + 16 hex characters is only 64 bits. It is cheap to use 16 random
   bytes / 32 lowercase hex characters for a long-lived exported lookup handle.
   Update examples, regexes, bounds, collision handling, and entropy table.

4. **Freeze one actual at-rest hash contract.**
   R1 silently changed recovery hashing to unspecified Argon2id. No parameters,
   salt representation, versioning, migration, or unauthenticated CPU-DoS
   budget are defined. These secrets already have >128 bits of uniform entropy;
   a fixed exact SHA-256/HMAC-SHA-256-style contract is sufficient if selected,
   while Argon2id is acceptable only with complete parameters and limiter-before-
   KDF behavior. Pick one and make recovery, Telegram code, control token, dummy
   verification, schema fields, and tests internally consistent. Never store
   plaintext.

5. **Validate the exact 30-symbol alphabet.**
   `^[A-Z2-9]{27}$` accepts `I`, `L`, `O`, `U` and other letters excluded from
   the generator. Use an exact character class or explicit membership check.
   Keep the corrected rejection sampler and 27-character length.

6. **Correct limiter claims and ordering.**
   A syntactically valid fake `rec_...` is still arbitrary and can create an LRU
   key; format validation does not bound keys to database rows. The explicit
   10,000-entry LRU cap does. Say that accurately. Apply the source-IP limiter
   to every bounded, syntactically valid recovery attempt before any expensive
   secret verification, including unknown IDs, while retaining a dummy equal-
   work verification path. Define what survives process restart as an accepted
   Phase-1 limitation.

7. **Freeze the Phase-1 Telegram trust boundary to the implementation we have.**
   The current coordinator uses authenticated TLS long polling in-process. Make
   consume an in-process service method receiving a principal derived from that
   Update. Do not leave an alternative public/internal HTTP endpoint with a
   vaguely “short-lived service credential” for developers to invent. A future
   split adapter requires its own authenticated protocol decision. Remove the
   inaccurate claim that the Update object itself is cryptographically bound;
   trust comes from the authenticated Bot API transport and protected bot token.

8. **Resolve link issuance policy without a false compatibility claim.**
   The current code lets all legacy members use `/share`, while the prose role
   document and new app-control model differ. For this Phase-1 app link contract,
   freeze: an active `primary` or `companion` app actor with a valid control
   token may issue either `companion` or `satellite`; `primary` can never be
   granted; satellite/revoked/left/disabled contexts cannot issue. This keeps
   optional Telegram usable for companion installations and caps authority below
   primary. Remove the unsupported “companion may grant equal but not lower”
   rationale.

9. **Finish HTTP and credential hygiene.**
   Require HTTPS outside loopback tests and `Cache-Control: no-store` on every
   secret-bearing request/response path (creation, rotation, link issuance,
   recovery consume), with request bodies excluded from access/error logs.
   Define invalid/revoked bearer tokens as `401 unauthorized`; reserve generic
   `credential_invalid` for unauthenticated secret consumes. The bot must never
   echo a link code and should best-effort delete the user's consumed code
   message where Telegram permits it.

10. **Remove unsupported fact-check assertions.**
    RFC 7235/FIDO2 do not, as cited, prove this exact retry protocol. Explain the
    protocol from its invariants or cite a source that directly supports the
    claim. Likewise distinguish mathematically exact entropy facts from policy
    choices. Keep direct links for every external factual assertion.

11. **Leave one authoritative outcome resource.**
    The R1 resubmission added `research.md` but left the stale
    `recovery-telegram-contract.md` attached with different bytes. Replace or
    remove the stale outcome so downstream agents cannot select the obsolete
    contract; the single outcome must match the working research file exactly.

## Resubmission

- Amend the single authoritative contract and its attached outcome.
- Preserve the corrected unbiased 27-character generation, atomic/idempotent
  consume, current-role recovery, generic secret errors, and trusted Telegram
  principal concept.
- Update downstream task impact, examples, schemas, and tests consistently.
- Keep all implementation source untouched.
- Reattach byte-identical outcome, retain the completed checklist only if every
  item remains true, and return to `to-review`, never `done`.
