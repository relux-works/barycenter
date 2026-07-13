# Root review round 1 — recovery and Telegram-link contract

Date: 2026-07-12  
Task: `TASK-260712-3v1k7q`  
Verdict: changes required; the current contract contains security and
recoverability defects that would be expensive to fix after implementation.

## Blocking findings

1. **The entropy claim is false.**
   The proposed alphabet has 30 symbols. Even perfectly uniform selection gives
   only `26 * log2(30) = 127.58` bits, not 130. The existing
   `randomCode` implementation uses `byte % 30`, which introduces additional
   bias because 256 is not divisible by 30. Use either at least 27 uniformly
   sampled characters from this alphabet (rejection sampling; 132.49-bit
   ceiling) or an unbiased 32-symbol encoding of 128 random bits. Correct every
   example and explicitly forbid reusing modulo-biased generation. This applies
   independently to recovery secrets and Telegram link codes.

2. **Recovery can permanently lock a user out after a lost HTTP response.**
   The current transaction burns the only recovery secret, revokes old control
   tokens, and returns a newly generated control token only in the response. If
   commit succeeds and the response is lost, retry fails as used and the new
   token is gone. Freeze a retry-safe protocol. A simple candidate is a
   client-generated 256-bit replacement control token included in the request:
   the server atomically stores only its hash, while the client already owns the
   plaintext if the response is lost. An equivalently safe idempotent/encrypted
   handoff is acceptable, but storing replayable plaintext server-side is not.
   Define retry and race behavior explicitly.

3. **Recovery incorrectly hard-codes `role: primary`.**
   An installation can be companion-owned. Return its current active membership
   role (or omit the field), never promote it. Freeze behavior for a revoked
   installation/actor, a left membership, and a disabled/revoked orbit: recovery
   must not resurrect or escalate them, and secret-facing failure remains the
   generic envelope. Clarify whether one installation can have more than one
   active membership; response types must match the actual integer/opaque ID
   model used by the additive migration.

4. **Telegram consume has no trustworthy caller boundary.**
   A public endpoint must never trust `telegram_user_id`, display name, or chat
   type from an unauthenticated JSON body. Prefer an in-process service method
   whose principal comes from a verified Telegram update. If an HTTP endpoint
   is retained, require a dedicated adapter/service credential or equivalent
   internal authentication and derive Telegram identity/context only from that
   trusted adapter. Specify that node/control tokens cannot impersonate a
   Telegram principal.

5. **The promised uniform JSON error envelope is missing.**
   Freeze the exact response body schema, content type, stable machine code, and
   public message for `invalid_request`, `unauthorized`,
   `insufficient_capability`, `credential_invalid`, conflicts, and rate limits.
   Unknown/expired/used/revoked/race-loser secret failures must have the same
   status, body shape, public text, and no materially different fast path.
   Define `Retry-After` units and response. Add bounded input lengths and a
   bounded limiter-key strategy so arbitrary fake `recovery_id` values cannot
   create unbounded state.

6. **Authorization to issue Telegram links is unspecified.**
   `control token only` is insufficient because `ActorContext` also has an
   active membership and role. Freeze who may issue `companion` and `satellite`
   links, including companion behavior, revoked/left actors, and disabled
   orbits. Do not silently turn Telegram linking into a broader invite privilege
   than the existing primary `/share` policy. Also define active versus historic
   membership for same-orbit/foreign-orbit conflicts and actor reuse.

7. **Opaque identifiers and credential sizes are underdefined.**
   Define `recovery_id` format/entropy and normalization, exact JSON scalar types
   for orbit/actor/Telegram IDs, and the replacement control token's 256-bit
   floor. Prefixes and formatting do not count toward entropy. Hash comparison
   and transaction rules must remain constant-time/atomic where applicable.

8. **The board checklist was left entirely unchecked.**
   On resubmission, check only items actually satisfied after the amended note is
   attached byte-identically. The researcher still returns `to-review`; root
   alone may mark `done`.

## Required resubmission shape

- Amend the existing contract so downstream schema/API/client tasks have one
  authoritative answer.
- Preserve the good one-time-display, control-only, generic replay/conflict, and
  secret-redaction decisions.
- Add the retry-safe recovery handshake, trusted Telegram principal boundary,
  exact error envelope, role authorization matrix, corrected entropy, and
  lifecycle/revocation cases.
- Keep source code untouched.
- Reattach byte-identical outcome content, complete the board checklist, and
  return the task to `to-review`, never `done`.
