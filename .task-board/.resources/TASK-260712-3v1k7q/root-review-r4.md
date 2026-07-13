# Root review round 4 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: rejected. R3 fixed the broad design, but the resulting byte-level hash
contract and pending-credential state machine are still unsafe to implement.

## Blocking corrections

1. **Freeze the exact token bytes that are hashed; the current claim is false.**
   `hashToken()` computes `SHA-256([]byte(token))`. Existing node tokens are
   64-character lowercase hexadecimal strings, so `slots.token_hash` is the
   digest of the **64 ASCII bytes**, not the digest of the 32 bytes obtained by
   hex-decoding that string. Sections 3, 5.2 and the compatibility claims
   currently prescribe the latter while saying it is identical to live code.
   That would make legacy node-token lookup fail if implemented uniformly.
   Use one exact, test-vector-backed contract: the minimal compatible choice is
   SHA-256 over the canonical token string bytes for node and control tokens,
   and SHA-256 over the normalized canonical string bytes for recovery/link
   secrets. Freeze lowercase-hex `TEXT` digests for the additive tables instead
   of leaving `TEXT` versus `BLOB` to downstream code. Include at least one
   concrete input/digest test vector and a legacy `LookupToken` compatibility
   test requirement.

2. **An attempted pending credential must never be auto-deleted from a
   snapshot or a generic response.**
   The revised client still deletes it on `400`, and deletes it after an earlier
   probe returned `401` followed by recovery `403`. Both are unsafe once any
   recovery request may have reached the server: an older timed-out/in-flight
   request can commit after the probe, or the user can mistype the secret after
   that commit. A probe result is a point-in-time observation, not cancellation
   of an in-flight mutation. Define protected pending state with an explicit
   `ever_sent`/uncertain marker. Once sent, keep and reuse the same candidate on
   every `400`, `403`, `429`, `5xx`, timeout, cancellation and restart until it
   is (a) authenticated and promoted, (b) superseded by a separately confirmed
   active credential, or (c) explicitly destroyed after the existing strong
   user warning. Automatic deletion is safe only for a candidate proven never
   sent. Update every flow, table, answer and test; do not retain the current
   step 5 or step 7d deletion branches.

3. **Make the actor-context probe a complete credential-validity protocol.**
   The endpoint returns authenticated `403 insufficient_capability` for a valid
   token whose membership is left or orbit disabled, but the pending-client
   algorithm handles only `200`, `401`, and network failure. A `403` proves the
   candidate authenticated and therefore must preserve/promote it (while the
   UI reports the unavailable actor/orbit context); it must never fall into an
   invalid or discard path. Freeze handling of every status (`200`, authenticated
   `403`, `401`, `429` if ever introduced, `5xx`, transport failure) and specify
   what metadata is retained when a valid credential has no active context.
   Ensure `401` cannot by itself erase an `ever_sent` candidate for the race in
   item 2.

4. **Freeze the additive/coexistence schema rather than describing a replacement
   migration.**
   The product spec requires existing tables to remain until cleanup and the
   schema task requires rollback to the previous coordinator with the feature
   flag off. State explicitly that legacy `members` and `slots` remain intact;
   backfill is idempotent and never deletes/rewrites their role or token rows.
   Legacy slots cannot receive newly minted control/recovery secrets during
   backfill because there is no plaintext delivery channel, so those credential
   fields must have a precise nullable/unprovisioned state. Freeze the minimum
   uniqueness needed to prevent identity drift: one Telegram actor per external
   Telegram ID, one installation actor per slot reference, unique code/hash
   lookup handles, and at most one active Phase-1 membership per actor. State
   how a slot's installation actor and role are derived from `paired_by`,
   including orphan/inconsistent legacy rows. Downstream migration code must not
   invent these security decisions.

5. **Make recovery state checks and the single-winner transition explicit.**
   The note promises one winner but the server algorithm merely says “atomic
   transaction” after reading `consumed_at`; freeze a conditional write or
   equivalent serialized transaction whose affected-row result selects the
   winner, followed by the same-token idempotency check for losers. Apply the
   revoked/left/disabled checks consistently to both first consume and
   idempotent replay, and state whether a valid candidate for a subsequently
   disabled/left context is authenticated by the probe as required in item 3.

## Resubmission

- Amend the one authoritative note and outcome byte-identically; do not edit
  product source.
- Preserve all accepted R1–R3 decisions: integer IDs, unbiased 27-character
  human secrets, client-generated replacement token, SHA-256 hash-only storage,
  role preservation, in-process Telegram principal, exact error envelope,
  concurrency-safe attempt reservation, HTTPS/no-store/redaction, and the link
  authorization matrix.
- Add the exact hash test vector, pending-state transition table, probe response
  table, additive migration/coexistence rules, and conditional consume rule to
  downstream tasks/tests.
- Return to `to-review`, never `done`.
