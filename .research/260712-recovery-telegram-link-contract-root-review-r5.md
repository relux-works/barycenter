# Recovery and Telegram Link Contract

Date: 2026-07-12
Task: `TASK-260712-3v1k7q`
Status: Contract note for downstream implementation (revised per root reviews R1–R5)

---

## Key Takeaways

- Recovery uses a **client-generated 256-bit replacement control token**. The
  client writes it to Keychain/DPAPI as a protected **pending credential** with
  an `ever_sent` marker **before sending** the destructive consume request. The
  server stores only its SHA-256 hash. Once `ever_sent` is true, the pending
  token is **never auto-deleted** regardless of any response (`400`, `403`,
  `429`, `5xx`, timeout, crash). It is removed only when (a) authenticated and
  promoted, (b) superseded by a separately confirmed active credential, or
  (c) explicitly destroyed after a strong user warning.
- **All identifiers use existing integer types.** `orbit_id` is the existing
  `orbits.id INTEGER` (JSON number). `actor_id` is a new
  `actors.id INTEGER PRIMARY KEY` (JSON number). No `orb_`/`act_` opaque string
  IDs are introduced. `recovery_id` is a new 128-bit lookup handle
  (`rec_` + 32 hex chars).
- **One at-rest hash contract** applies to every secret: unkeyed SHA-256 over
  the **canonical string bytes** of each token/secret (the same operation as
  `hashToken()` in live code). No HMAC key, no Argon2id, no per-secret salt.
  High-entropy inputs (>128 bits uniform) make brute-force infeasible. Storage
  format for all additive tables is **lowercase hex TEXT** (64 characters),
  matching existing `slots.token_hash`.
- **Schema ownership is separated.** `actors` and `memberships` are identity
  and role (no secret columns). `installation_credentials` owns the current
  control token hash, recovery state, and a foreign-key reference to the
  authoritative `slots` row for the node token hash. Existing `slots` and
  their token hashes remain valid with no migration or duplication.
- **Legacy coexistence.** Existing `members` and `slots` tables remain intact.
  Backfill is idempotent and never deletes/rewrites their role or token rows.
  Rollback to the previous coordinator with the feature flag off is safe.
- Recovery response returns the actor's **current active membership role**, never
  hard-coded `primary`. Phase 1: one membership per actor, flat response (no
  array). Revoked/left/disabled actors fail with the generic error.
- Telegram consume is an **in-process service method** whose principal comes from
  a verified Telegram Bot API `Update` received over authenticated TLS long
  polling. No public HTTP endpoint. No separate adapter credential.
- The **authorization matrix** for link issuance: active `primary` or
  `companion` with a valid control token may issue either `companion` or
  `satellite` links. `primary` role is never granted via link. Satellite, revoked,
  left, and disabled contexts cannot issue.
- The **uniform JSON error envelope** is frozen with exact codes and messages.
  Secret-facing failures on unauthenticated endpoints use one
  `403 credential_invalid` code with no timing or content difference.
  Invalid/revoked bearer tokens on authenticated endpoints use
  `401 unauthorized`. Valid tokens lacking active role or orbit authority use
  `403 insufficient_capability`.
- **HTTPS required** outside loopback tests. `Cache-Control: no-store` on every
  secret-bearing path. Request bodies excluded from access/error logs. The bot
  never echoes a link code and best-effort deletes consumed code messages.
- **Attempt limits are concurrency-safe.** Every syntactically valid consume
  request atomically reserves an attempt counter BEFORE any hash work.
  Concurrent requests cannot all slip through a pre-check window. Counts are
  all attempts (not only failures) for simplicity and safety.
- **Recovery consume is bound to the exact recovery generation.** The conditional
  write predicates on `recovery_id` and `consumed_at IS NULL` — not merely
  `actor_id`. A concurrent
  `/recovery/rotate` that replaces the row cannot be overwritten by a stale
  consume request because the old `recovery_id` no longer matches. SQLite
  serializes writers via database-level locking; the conditional `UPDATE`
  statement and its predicates provide the invariant.
- **Single-row recovery rotation model.** Each `installation_credentials` row
  has exactly one current recovery generation. Rotation atomically overwrites
  `recovery_id`, `recovery_secret_hash`, and resets `consumed_at = NULL`. There
  is no multi-row generation history. The old `recovery_id` becomes permanently
  invalid. Idempotent replay of a consumed secret is valid only until the next
  rotation replaces it.
- **Telegram link consume is a rollback-safe transaction.** Actor resolution,
  conflict checks, code reservation, membership creation, legacy dual-write,
  and audit happen inside one SQLite transaction. Any failure (membership
  constraint violation, concurrent winner, revoked actor) rolls back code
  consumption so the code remains unconsumed. The conditional code write
  includes expiry, `invalidated_at IS NULL`, and `consumed_at IS NULL`
  predicates.
- **No node-token control escalation.** A node token grants playback, heartbeat,
  and scoped media download only. It MUST NOT provision control tokens or
  recovery material. Legacy unprovisioned installations obtain control authority
  through a separately authorized flow (device invite from primary/companion,
  explicit Telegram-owner authorization, or another frozen proof).
- **Database-enforced uniqueness** for one-active-membership-per-actor: a partial
  unique index `ON memberships(actor_id) WHERE left_at IS NULL` replaces the
  prior application-level check. This is race-proof.
- **Dual-write coexistence.** New or reactivated Telegram memberships
  transactionally keep the legacy `members` table consistent. Feature-flag-off
  and the previous coordinator see the same role and membership data.
- **Conservative backfill.** Orphan/inconsistent legacy slots (no matching
  `paired_by` member) backfill as `satellite` — the lowest-capability role that
  cannot issue links or exercise control. Explicit authorized repair is required
  to upgrade.

---

## Source Baseline

- `docs/spec-self-contained-audio.md:317-391` — node/control split, one-time
  recovery display, control-only recovery, unrecoverable-loss wording.
- `docs/spec-self-contained-audio.md:622-638` — Phase 1 HTTP endpoint table.
  The additive `GET /v1/actor/context` probe is not in this table yet.
- `docs/spec-self-contained-audio.md:719-727` — actors/memberships logical
  schema. The spec places `control_token_hash` on `actors`; this contract
  separates it into `installation_credentials` as a refinement — actors hold
  identity only, credentials hold secrets.
- `coordinator/internal/store/orbits.go:20-26` — live schema:
  `orbits(id INTEGER PRIMARY KEY AUTOINCREMENT, ...)`.
- `coordinator/internal/store/orbits.go:28-36` — live schema:
  `members(orbit_id INTEGER, tg_user_id INTEGER, ..., PRIMARY KEY(orbit_id, tg_user_id))`.
- `coordinator/internal/store/orbits.go:37-46` — live schema:
  `slots(orbit_id INTEGER, slot TEXT, token_hash TEXT, ..., PRIMARY KEY(orbit_id, slot))`.
  `token_hash` is hex-encoded unkeyed SHA-256 of the token **string** bytes.
- `coordinator/internal/store/orbits.go:88-98` — Go types: `Orbit.ID int64`,
  `Member.OrbitID int64`, `Member.TGUserID int64`.
- `coordinator/internal/store/orbits.go:105-108` — existing `hashToken()`:
  `sha256.Sum256([]byte(token))`, hex-encoded. The input is the token string
  as raw bytes (64 ASCII characters for hex tokens), NOT hex-decoded binary.
  This is the authoritative at-rest hash for `slots.token_hash` and the model
  for all new hashes.
- `coordinator/internal/store/orbits.go:119-130` — existing modulo-biased
  `randomCode()`: `byte % 30` on a 30-symbol alphabet, 8 characters. This
  function MUST NOT be reused for new secret material.
- `coordinator/internal/store/orbits.go:536-541` — existing atomic
  `UPDATE ... WHERE used_at IS NULL` winner-takes-code pattern with
  `RowsAffected()` check.
- `docs/v2-multitenant-design.md:27-29` — `companion` is the default invitee
  role; `satellite` is bot-only.
- `.task-board/.resources/STORY-260712-2ve1c8/p1-identity-model.puml` —
  `AppInstallationCredential` with `recovery_secret_hash`,
  `TelegramLinkCode` with `desired_role`, `consumed_at`, and
  `Actor <-> Membership` cardinality.
- `.task-board/.resources/STORY-260712-2ve1c8/p1-onboarding-flows.puml` —
  recovery reissues control credential only; Telegram consume uses verified
  identity from the bot adapter.
- `docs/analysis/p1-root-review-amendments.md:22-26` — uniform errors, attempt
  limits, atomic single-use semantics, complete secret redaction.

---

## Fact-Check

1. **Entropy calculation (mathematical fact).**
   The alphabet `ABCDEFGHJKMNPQRSTVWXYZ23456789` has exactly 30 symbols.
   `log2(30) = 4.907`. To exceed 128 bits: `ceil(128 / 4.907) = 27` characters.
   `27 * log2(30) = 132.49` bits. This is an exact computation; 27 characters
   is the minimum length for >128 bits from a 30-symbol uniform alphabet.

2. **Rejection sampling (mathematical fact).**
   `floor(256 / 30) = 8`, so `limit = 30 * 8 = 240`. A random byte `b` is
   accepted if `b < 240`; the selected symbol is `alphabet[b / 8]`. Each of
   the 30 symbols maps to exactly 8 of the 240 accepted byte values, so
   selection is perfectly uniform. Rejection rate = `(256 - 240) / 256 = 6.25%`.
   Expected draws per character = `256 / 240 ≈ 1.067`. For 27 characters:
   ≈28.8 bytes from `crypto/rand`.

3. **SHA-256 for high-entropy secrets (design rationale, not external claim).**
   Secrets with >128 bits of uniform entropy cannot be brute-forced regardless
   of hash function choice. Even knowing the input space (30^27 ≈ 2^132.49
   possible values), finding the preimage of a given SHA-256 hash requires
   ≈2^132 operations, which is computationally infeasible. Unkeyed SHA-256 is
   chosen because: (a) it is the existing hash function for `slots.token_hash`,
   ensuring backward compatibility with no migration; (b) it requires no
   server-side key management, eliminating key-compromise, key-rotation, and
   key-file operational failure modes; (c) it is fast enough for
   unauthenticated endpoints without creating a CPU-DoS vector; (d) it is
   preimage-resistant and collision-resistant for all practical purposes.
   Consistent with
   [NIST SP 800-63B §5.1.2.1](https://pages.nist.gov/800-63-4/sp800-63b.html#memsecretver)
   guidance that look-up secrets with sufficient entropy may use approved
   one-way functions without slow KDFs.

4. **Telegram Bot API trust model (corrected per R2).**
   The coordinator receives Telegram updates via authenticated TLS long polling
   using the protected bot token. The `from.id` field in a received `Update` is
   server-asserted by Telegram's infrastructure. Trust comes from the
   authenticated Bot API transport and the secrecy of the bot token, NOT from
   any cryptographic signature on the `Update` object itself (there is none
   for long-polling; webhook mode has a separate optional secret-token header).
   An in-process consumer of these updates can therefore trust `from.id` as a
   Telegram principal.
   Source: [Telegram Bot API — Getting updates](https://core.telegram.org/bots/api#getting-updates)

5. **Constant-time comparison (referenced API).**
   Go `crypto/subtle.ConstantTimeCompare` performs fixed-time byte comparison
   to prevent timing side-channels in authentication paths.
   Source: [Go crypto/subtle](https://pkg.go.dev/crypto/subtle)

6. **OWASP rate-limiting guidance (policy reference).**
   OWASP Authentication Cheat Sheet recommends generic failure responses, login
   throttling with configurable thresholds, and bounded key spaces to prevent
   state-exhaustion attacks on the limiter.
   Source: [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)

7. **Idempotent recovery retry (design invariant, not external claim).**
   The protocol survives a lost HTTP response because: (a) the client holds the
   replacement control token before sending the request; (b) the server stores
   only the SHA-256 hash of that token; (c) on retry with the same
   `(recovery_id, recovery_secret, replacement_control_token)` tuple, the server
   detects the consumed secret, verifies the token hash matches, and returns
   success without mutation. This is an invariant of this specific protocol.

8. **Legacy token hash compatibility (verified against source).**
   The existing `hashToken()` at `coordinator/internal/store/orbits.go:105`
   uses `sha256.Sum256([]byte(token))` — SHA-256 of the token's **string
   bytes**. For existing node tokens, `token` is a 64-character lowercase hex
   string produced by `randomHex(32)`. The hash input is the 64 ASCII bytes
   (`0x30`–`0x39`, `0x61`–`0x66`), NOT the 32 bytes obtained by hex-decoding
   the string. `slots.token_hash TEXT` stores the resulting 64-character
   lowercase hex digest. This was verified by reading the source at lines
   105–108 and 581–583.

9. **SQLite writer serialization (corrected per R5).**
   SQLite uses **database-level locking**, not row-level locking. In WAL mode,
   only one writer can hold the write lock at a time; concurrent writers are
   serialized. The conditional `UPDATE` statement with its predicates provides
   the application-level invariant (e.g., `WHERE recovery_id = ? AND
   consumed_at IS NULL`). The combination of database-level writer serialization
   and the conditional `UPDATE` predicate ensures exactly one winner.
   Source: [SQLite File Locking](https://www.sqlite.org/lockingv3.html),
   [WAL mode](https://www.sqlite.org/wal.html)

10. **Test vector (computed, reproducible).**
   Input string: `"0000000000000000000000000000000000000000000000000000000000000000"`
   (64 ASCII zero characters, 64 bytes).
   `SHA-256(input)` =
   `60e05bd1b195af2f94112fa7197a5c88289058840ce7c6df9693756bc6250f55`
   This matches `hashToken("000...0")` and MUST be reproduced by any new hash
   implementation to confirm compatibility with `LookupToken`.

   Recovery secret test vector:
   Input string: `"ABCDEFGHJKMNPQRSTVWXYZ23456"` (27 ASCII uppercase chars,
   27 bytes).
   `SHA-256(input)` =
   `e45d6091f70eeb484d8b9fe2e4a9067d0159b336298c9a5f30804f592c3e824d`

---

## Frozen Contract

### 1. Secret Generation

All recovery secrets and Telegram link codes MUST be generated using the
following algorithm. The existing `randomCode()` at
`coordinator/internal/store/orbits.go:119` (modulo-biased, 8 characters) is
forbidden for new secret material; it may continue to serve legacy pairing
codes until those are migrated.

**Alphabet:**

```
ABCDEFGHJKMNPQRSTVWXYZ23456789
```

30 symbols. Excluded: `I` (confused with `1`), `L` (confused with `1`),
`O` (confused with `0`), `U` (confused with `V`), `0` (confused with `O`),
`1` (confused with `I`/`L`).

**Algorithm: `generateSecret(length int) string`**

```
alphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"  // 30 symbols, compile-time constant
limit   = 240                                 // 30 * 8; largest multiple of 30 <= 256
result  = []
while len(result) < length:
    b = one byte from crypto/rand
    if b < limit:
        result.append(alphabet[b / 8])        // integer division
    // else: reject and retry (6.25% rejection rate)
return string(result)
```

**Lengths and entropy:**

| Material | Length | Entropy (bits) |
|---|---|---|
| Recovery secret | 27 chars | 132.49 |
| Telegram link code | 27 chars | 132.49 |

**Input normalization:** before comparison, strip ASCII whitespace and hyphens.
Convert to uppercase. The canonical form is uppercase, no separators, exactly
`length` characters.

**Validation regex (exact alphabet):**

```
^[ABCDEFGHJKMNPQRSTVWXYZ2-9]{27}$
```

This character class matches exactly the 30 symbols in the generator. It
rejects `I`, `L`, `O`, `U`, `0`, `1` — unlike `[A-Z2-9]` which would
incorrectly accept them.

### 2. Identifiers and Scalar Types

| Identifier | JSON type | Format | Notes |
|---|---|---|---|
| `orbit_id` | `number` (integer) | Existing `orbits.id INTEGER PRIMARY KEY AUTOINCREMENT` | Live schema, Go `int64`. No `orb_` strings. |
| `actor_id` | `number` (integer) | New `actors.id INTEGER PRIMARY KEY` | Additive table. Go `int64`. No `act_` strings. |
| `recovery_id` | `string` | `rec_` + 32 lowercase hex chars | 128-bit lookup handle. Not a secret. |
| `telegram_user_id` | `string` | Decimal digits | Telegram-asserted. String in JSON for 64-bit safety; maps to existing `INTEGER` column. |
| `control_token` | `string` | 64 lowercase hex chars | 256-bit. Client holds plaintext; server holds SHA-256 hash of the 64-char string. |
| `node_token` | `string` | 64 lowercase hex chars | 256-bit. Same hash pattern. |
| `recovery_secret` | `string` | 27 uppercase from safe alphabet | 132.49-bit. Shown once; server holds SHA-256 hash of normalized canonical form. |
| `link_code` | `string` | 27 uppercase from safe alphabet | 132.49-bit. Same generation and hash. |

**`recovery_id` details:**

- Generated server-side: 16 random bytes from `crypto/rand`, hex-encoded to 32
  lowercase hex characters, prefixed with `rec_`.
- Regex: `^rec_[0-9a-f]{32}$`. Max 36 characters.
- 128-bit entropy makes collision probability negligible. On the
  astronomically unlikely collision, generate-and-retry.
- The 4-character `rec_` prefix does NOT count toward entropy.
- `recovery_id` is a non-secret lookup handle. It MAY be persisted by the
  client alongside non-secret metadata.

**No new public ID system for existing entities.** `orbit_id` and `actor_id`
use the existing SQLite `INTEGER PRIMARY KEY` type. No opaque prefixed strings,
no mapped columns, no backfill. The additive `actors` table uses
`INTEGER PRIMARY KEY` (SQLite rowid alias) with Go `int64`.

### 3. At-Rest Hash Contract

**One hash function for all secrets:** unkeyed SHA-256.

**Hash computation — the canonical rule:**

```
hash = hex.EncodeToString(SHA-256([]byte(canonical_string)))
```

The input to SHA-256 is always the **string bytes** (UTF-8/ASCII) of the
canonical token or secret representation. This is the same operation as the
existing `hashToken()` at `coordinator/internal/store/orbits.go:105`:

```go
func hashToken(token string) string {
    h := sha256.Sum256([]byte(token))
    return hex.EncodeToString(h[:])
}
```

| Secret type | Canonical string | `[]byte(...)` length |
|---|---|---|
| `node_token` | 64 lowercase hex chars (as produced by `randomHex(32)`) | 64 bytes |
| `control_token` | 64 lowercase hex chars (as produced by client CSPRNG + hex) | 64 bytes |
| `recovery_secret` | Normalized: 27 uppercase chars from the safe alphabet | 27 bytes |
| `link_code` | Normalized: 27 uppercase chars from the safe alphabet | 27 bytes |

**Critical compatibility note:** the hash input for hex tokens is the 64-byte
ASCII representation, NOT the 32-byte binary obtained by hex-decoding the
string. The existing `LookupToken` path at line 618 passes the raw token string
to `hashToken()` and queries `slots.token_hash`. Any new hash implementation
MUST produce the same digest for the same token string; hex-decoding before
hashing would break legacy node-token authentication.

**Test vectors (computed, any implementation MUST reproduce):**

| Input (canonical string) | SHA-256 hash (lowercase hex, 64 chars) |
|---|---|
| `"0000000000000000000000000000000000000000000000000000000000000000"` (64 zeros) | `60e05bd1b195af2f94112fa7197a5c88289058840ce7c6df9693756bc6250f55` |
| `"ABCDEFGHJKMNPQRSTVWXYZ23456"` (27 chars, safe alphabet) | `e45d6091f70eeb484d8b9fe2e4a9067d0159b336298c9a5f30804f592c3e824d` |

**`LookupToken` compatibility test requirement:** any new code that authenticates
a node token MUST pass the following test: generate a token with `randomHex(32)`,
store `hashToken(token)` in the database, and verify that authentication with
the same token string succeeds. This confirms the hash-input convention is
consistent with the existing `LookupToken` path.

**No server-side key management.** Unlike HMAC, unkeyed SHA-256 requires no
key file, no key generation at first launch, no key rotation, and no key
compromise recovery. The security margin comes from the input entropy
(>128 bits uniform), which makes brute-force infeasible regardless of hash
function. A database-only compromise reveals hashes that cannot be reversed in
any computationally feasible time.

**Storage format for all additive tables:** lowercase hex `TEXT`, 64 characters.
This matches `slots.token_hash`. Column names: `recovery_secret_hash`,
`code_hash`, `control_token_hash`. The schema task MUST NOT choose BLOB or any
other representation for additive tables; consistent TEXT enables the same
`hashToken()` function and query patterns across all credential types.

Existing `slots.token_hash TEXT` (64 hex characters, unkeyed SHA-256 of the
string bytes) remains valid with no migration. It is the authoritative store
for node token hashes; `installation_credentials` references the slot row, not
a duplicated hash.

**Plaintext secrets are NEVER stored.** Not as a second column, not
temporarily, not in a log.

**Comparison:**

- Always use `crypto/subtle.ConstantTimeCompare` on the fixed 32-byte digests
  (hex-decode both computed and stored 64-char hex strings to `[32]byte` before
  comparison).
- The comparison is always on the raw digest bytes, regardless of TEXT storage
  format.

**Dummy verification (timing equalization):**

- At startup, precompute: `dummy_hash = hashToken(string(make([]byte, 64)))` —
  the hash of 64 zero bytes as a string. In practice, this is a fixed constant.
- For unknown `recovery_id` values (no database row): compute
  `hashToken(submitted_secret)` and constant-time compare against
  `dummy_hash`. This ensures unknown-ID attempts take the same time as
  wrong-secret attempts.
- Same pattern for unknown `link_code` rows.

### 4. Recovery Material Lifecycle

1. `POST /v1/onboarding/orbits` and `POST /v1/recovery/rotate` MUST return:

   ```json
   {
     "recovery_id": "rec_a1b2c3d4e5f6789001a2b3c4d5e6f789",
     "recovery_secret": "ABCDEFGHJKMNPQRSTVWXYZ23456",
     "shown_once": true
   }
   ```

2. `recovery_id` is a stable non-secret handle for one installation
   credential row. The client MAY persist `recovery_id`, `orbit_id`, issuance
   timestamps, and a local "user confirmed backup" flag.
3. The client MUST NOT silently persist `recovery_secret` beside node/control
   credentials, in app-private files, Keychain/DPAPI, logs, telemetry,
   analytics, clipboard history, pasteboard history, or crash reports.
4. Any explicit copy/save/export flow MUST package `recovery_id` and
   `recovery_secret` together; a clean install will otherwise lack the
   non-secret handle needed for lookup.
5. `recovery_secret` is single-use on successful recovery. The server stores
   only a SHA-256 hash; the plaintext secret is never replayable by a
   server read.
6. No read endpoint may ever return the current recovery secret. The only
   legal plaintext returns are initial orbit creation and explicit
   authenticated rotation.
7. Loss of the sole installation plus an unsaved recovery secret is
   **unrecoverable**. This MUST be stated verbatim in create/rotate UI copy
   and in the saved-export flow.

### 5. `POST /v1/recovery/consume`

Auth: none (unauthenticated). Rate limited by source IP and `recovery_id`.

**Request:**

```json
{
  "recovery_id": "rec_a1b2c3d4e5f6789001a2b3c4d5e6f789",
  "recovery_secret": "ABCDEFGHJKMNPQRSTVWXYZ23456",
  "replacement_control_token": "a3f8...64 hex chars total..."
}
```

**Success `200 OK`:**

```json
{
  "orbit_id": 42,
  "actor_id": 7,
  "role": "companion"
}
```

Phase 1: one active membership per actor. The response is a flat object with
the single active membership's orbit, actor, and role. No `memberships` array.

#### 5.1 Client-Side Pending Credential Protocol

The client MUST protect against lost HTTP responses, process/machine crashes,
and ambiguous server state after any request has been sent.

**Pending state record (Keychain/DPAPI):**

| Field | Type | Purpose |
|---|---|---|
| `recovery_id` | string | Lookup handle for this recovery attempt |
| `pending_control_token` | string (64 hex) | The replacement credential |
| `ever_sent` | boolean | Set to `true` immediately before the first network send; never reverted |

**Protocol:**

1. **Generate:** Client generates a 256-bit replacement control token (32 bytes
   from platform CSPRNG, hex-encoded to 64 characters).
2. **Persist as pending:** Client writes
   `{recovery_id, pending_control_token, ever_sent: false}` to
   Keychain (macOS) or DPAPI/Credential Locker (Windows) BEFORE sending the
   request. The `recovery_secret` is NOT written; it remains user-supplied.
3. **Mark sent:** Client sets `ever_sent = true` in the protected record.
4. **Send:** Client sends `POST /v1/recovery/consume` with
   `(recovery_id, recovery_secret, replacement_control_token)`.
5. **On success response (200):**
   a. Promote `pending_control_token` to the active `control_token` in
      Keychain/DPAPI.
   b. Store `orbit_id` and `actor_id` from the response.
   c. Delete the pending state record entirely.
   d. Delete any previously active control token for a different actor.
6. **On `400 invalid_request`:**
   a. If `ever_sent` is `false`: safe to delete the pending record (the request
      was rejected before any server transaction path). This branch is reachable
      only if client-side validation has a bug — normally format errors are
      caught before step 3.
   b. If `ever_sent` is `true`: **do NOT delete**. Keep the pending record.
      A `400` after send could result from a retry reaching the server after a
      prior timeout where the original request committed. Display the error,
      allow retry.
7. **On `403 credential_invalid`:**
   a. **Do NOT delete** the pending record regardless of `ever_sent` state.
      A `403` from recovery is never proof that the pending candidate is
      inactive — the user may have mistyped the recovery secret while the server
      already committed the replacement token from a prior in-flight attempt.
   b. Display a generic error. Suggest re-entering the secret.
8. **On `429 too_many_attempts`:**
   a. **Do NOT delete** the pending record.
   b. Display rate-limit message with `retry_after_seconds` from response.
9. **On `5xx` or network error or process crash before receiving a response:**
   a. **Do NOT delete** the pending record.
   b. On restart, the client detects pending state with `ever_sent = true`.
   c. Continue to step 10 (probe protocol).
10. **Pending state recovery (on restart or after any non-200 response with
    `ever_sent = true`):**
    a. **Probe:** call `GET /v1/actor/context` (§6) with the pending token as
       Bearer.
    b. Handle probe response per the **Probe Response Table** (§5.1.1).

**Invariant:** once `ever_sent` is `true`, the pending credential is never
auto-deleted. It persists across restarts, errors, and timeouts until one of
the three terminal conditions is met: (a) authenticated and promoted,
(b) superseded, or (c) user-confirmed destructive abandon.

#### 5.1.1 Probe Response Table

The actor-context probe (`GET /v1/actor/context` with pending token as Bearer)
has the following complete response handling:

| Probe status | Meaning | Action |
|---|---|---|
| `200 OK` | Token authenticated, active context | **Promote:** the pending token is the working credential. Move to active. Store returned `orbit_id`, `actor_id`, `role`. Delete pending record. |
| `403 insufficient_capability` | Token authenticated, but actor has no active context (left membership, disabled orbit) | **Promote with limited context:** the token IS valid (the server authenticated it). Move to active. The UI reports that the orbit/membership is currently unavailable. Retain the credential — context may be restored (orbit re-enabled, membership re-granted). |
| `401 unauthorized` | Token is definitively invalid on the server | The pending token was never committed, or was superseded. Ask user for `recovery_secret` and retry `POST /v1/recovery/consume` with the **same** `(recovery_id, recovery_secret, pending_control_token)` tuple. On retry 200: promote (step 5). On retry 403: the secret is also wrong/consumed. Inform user; keep pending record (do NOT delete — see race note below). |
| `429` (if ever introduced) | Rate limited | Retry probe after `retry_after_seconds`. Keep pending. |
| `5xx` | Server error | Retry probe with backoff. Keep pending. |
| Network failure | Cannot reach server | Retry probe with backoff. Keep pending. Ask user for `recovery_secret` if they want to attempt recovery directly. |

**Race note for `401` → retry `403` sequence:** A probe returning `401` is a
point-in-time observation. Between the probe and the recovery retry, a prior
in-flight request could commit the pending token. If recovery retry returns
`403`, it could mean (a) the secret was mistyped, or (b) the prior commit went
through and the secret is now consumed. The client MUST NOT delete the pending
record. The user can re-probe later to distinguish the cases.

**Discard/Cancel:**

- If the latest probe returned `401` AND `ever_sent` is `true` AND recovery
  retry also returned `403` (same session, no intervening network failures):
  the client MAY offer a Cancel action with a strong destructive-abandon
  warning: "If the server accepted this token from a prior attempt, deleting
  it means permanent loss of access." On user confirmation, delete the pending
  record.
- If `ever_sent` is `false`: safe to delete without confirmation (request was
  never sent).
- In all other cases (probe not completed, probe returned `403`/`5xx`/network
  error): Cancel MUST display the destructive-abandon warning before deletion.

#### 5.2 Server-Side Transaction

Processing order:

1. Parse and validate input format. Reject syntactically invalid input with
   `400 invalid_request`. No attempt counter is touched.
2. Source-IP attempt counter: atomically reserve (increment) and check. Reject
   if over limit with `429`. The increment and check MUST be a single atomic
   operation (e.g., `sync/atomic` or mutex-protected counter).
3. Per-`recovery_id` attempt counter: atomically reserve (increment) and check.
   Reject if over limit with `429`.
4. Check actor/orbit status: look up the credential row by `recovery_id`.
5. **If not found:** compute `hashToken(submitted_secret)` and constant-time
   compare against `dummy_hash`. Return `403 credential_invalid`.
6. **If found — check lifecycle state:**
   a. Look up the actor's `revoked_at`, membership's `left_at`, orbit's
      `status`. If revoked, left, or disabled: compute
      `hashToken(submitted_secret)` and constant-time compare against
      `dummy_hash` (do NOT compare against the real hash — this prevents
      confirming the secret is correct for an unusable context). Return
      `403 credential_invalid`.
7. **If found and secret not consumed (`consumed_at IS NULL`):**
   a. Compute `hashToken(submitted_secret)` and constant-time compare against
      stored `recovery_secret_hash`.
   b. If no match: return `403 credential_invalid`.
   c. If match — **generation-bound conditional write (single-winner):**
      ```sql
      UPDATE installation_credentials
      SET consumed_at = ?,
          control_token_hash = ?
      WHERE actor_id = ?
        AND recovery_id = ?
        AND consumed_at IS NULL
      ```
      The `recovery_id` predicate binds this consume to the exact recovery
      generation that was looked up. A concurrent `/recovery/rotate` that
      replaces the row's `recovery_id` causes this write to match zero rows
      instead of overwriting the new generation's `control_token_hash`.

      Check `RowsAffected()`:
      - If `rows == 1`: commit succeeded. This request is the winner.
        Write audit event (no plaintext). Return `200 OK` with
        `{orbit_id, actor_id, role}`.
      - If `rows == 0`: a concurrent request consumed first, or a concurrent
        rotation replaced the generation. Fall through to step 8 (reload and
        check).
8. **If conditional write returned `rows == 0`, OR the row was already
   consumed (`consumed_at IS NOT NULL`) at step 4:**
   a. **Reload the current row** by `actor_id`. Never compare against stale
      fields from the step-4 read — a concurrent rotation or consume may have
      changed `recovery_id`, `recovery_secret_hash`, or `control_token_hash`.
   b. If no row found (actor deleted): return `403 credential_invalid`.
   c. If the reloaded `recovery_id` differs from the submitted `recovery_id`:
      a rotation replaced the generation. Return `403 credential_invalid`. The
      client's secret cannot be verified against the new generation.
   d. Re-check lifecycle state (actor revoked, membership left, orbit disabled).
      If any is true: compute dummy comparison, return
      `403 credential_invalid`. This ensures a subsequently disabled context
      cannot replay even with correct material.
   e. Compute `hashToken(submitted_secret)` and constant-time compare against
      the **reloaded** `recovery_secret_hash`.
   f. If secret does not match: return `403 credential_invalid`.
   g. If secret matches: compute `hashToken(replacement_control_token)` and
      constant-time compare against the **reloaded** `control_token_hash`.
   h. If token hash matches: **idempotent success** — return `200 OK` with
      `{orbit_id, actor_id, role}`. No mutation.
   i. If token hash does not match: return `403 credential_invalid`. This
      prevents a different client from replaying with a different token.

Both the secret and the replacement token are verified on idempotent replay
(step 8e–8g) against **reloaded** row data. An attacker with only the pending
token (e.g., Keychain access without the recovery secret) cannot complete
recovery.

**Single-winner guarantee:** The generation-bound `WHERE recovery_id = ? AND
consumed_at IS NULL` conditional write with
`RowsAffected()` check is the serialization mechanism. SQLite serializes writers
via **database-level locking** (not row-level locking). The conditional `UPDATE`
statement and its predicates provide the invariant: exactly one concurrent writer
can set `consumed_at` from NULL to a value for a given `recovery_id`. The
loser's `RowsAffected() == 0` triggers a reload (step 8a) before any comparison,
ensuring stale fields from a pre-rotation read are never used.

**Consume-vs-rotate serialization:** If a consume request reads the row at step 4
and a concurrent rotation then overwrites `recovery_id` and
`recovery_secret_hash`, the consume's conditional write at step 7c matches zero
rows because the old `recovery_id` no longer exists. The reload at step 8a finds
a different `recovery_id` → step 8c returns `credential_invalid`. The new
generation's `control_token_hash` is never overwritten by the stale consume.

#### 5.3 Lifecycle Rules

- Recovery reissues **only the control credential**. It MUST NOT mint, rotate,
  transfer, or revoke the node token. If the installation still has its
  `node_token` locally, it continues working. If lost, the installation is in
  control-only state until explicit rebind.
- The `role` field reflects the actor's **current active membership role**. It
  is never hard-coded to `primary`. A companion installation recovers as
  companion.
- One app installation actor has **at most one active membership** per orbit
  in Phase 1.
- Recovery MUST fail with `403 credential_invalid` (generic) if any of:
  - The actor is revoked (`actors.revoked_at IS NOT NULL`).
  - The membership is left (`memberships.left_at IS NOT NULL`).
  - The orbit is disabled (`orbits.status = 'disabled'`).
  Recovery never resurrects or escalates a revoked/left/disabled state.
  The check applies to both first consume (step 6) and idempotent replay
  (step 8a).
- Successful recovery does NOT auto-rotate a new recovery secret. The fresh
  control token may call `POST /v1/recovery/rotate` if the user explicitly
  chooses to create new recovery material.
- Concurrent attempts: exactly one transaction may consume a given
  `(recovery_id, recovery_secret)` pair successfully via the generation-bound
  conditional write. Any concurrent loser or post-success replay returns
  `403 credential_invalid`, except the idempotent-retry case (step 8h).
- **Idempotency lifetime after rotation:** a consumed recovery secret's
  idempotent replay (step 8h) is valid only until the next
  `/recovery/rotate` replaces the row's `recovery_id` and
  `recovery_secret_hash`. After rotation, the old `recovery_id` is gone and
  retries with it fail at step 8c with `credential_invalid`. This is safe
  because the client already holds the promoted control token from the
  original successful consume.
- **Probe behavior for a subsequently disabled/left context:** If recovery was
  previously successful and the actor/orbit is later disabled or the membership
  left, the control token remains valid for authentication purposes. The probe
  returns `403 insufficient_capability` (confirming the token is valid but
  lacking active context). The client promotes and retains the credential; the
  UI reports the unavailable context. This is required by §5.1.1 item 3.

#### 5.4 Input Constraints

| Field | Validation | Max length |
|---|---|---|
| `recovery_id` | `^rec_[0-9a-f]{32}$` | 36 chars |
| `recovery_secret` | After normalization: `^[ABCDEFGHJKMNPQRSTVWXYZ2-9]{27}$` | 40 chars pre-normalization |
| `replacement_control_token` | `^[0-9a-f]{64}$` | 64 chars |
| Request body | Valid JSON | 512 bytes |

Any field exceeding bounds or failing format validation produces
`400 invalid_request` without touching attempt counters.

### 6. `GET /v1/actor/context`

Auth: node or control token (Bearer header).

**Success `200 OK`:**

```json
{
  "orbit_id": 42,
  "actor_id": 7,
  "role": "companion"
}
```

**Error `401 unauthorized`:** token is missing, malformed, or not found in any
active credential record.

**Error `403 insufficient_capability`:** token authenticates successfully (valid
credential row found) but the actor has no active usable context:

```json
{
  "error": {
    "code": "insufficient_capability",
    "message": "This token does not have the required capability.",
    "retry_after_seconds": null
  }
}
```

This status is returned when:
- The membership is left (`memberships.left_at IS NOT NULL`).
- The orbit is disabled (`orbits.status = 'disabled'`).
- The actor is revoked (`actors.revoked_at IS NOT NULL`) — returns `401`
  instead (revocation invalidates the token itself).

**Metadata retained on `403 insufficient_capability`:** the server does NOT
return `orbit_id`/`actor_id`/`role` in the error response. The client knows
the token is valid (it authenticated), but cannot derive context until the
membership/orbit is restored. The pending-credential protocol treats this as
"authenticated → promote" regardless of whether metadata is present.

This is a read-only endpoint. It performs no mutation. Its primary use is the
pending-credential probe (§5.1.1), but it is also a general-purpose
identity/health check for any token holder.

Rules:

- Both node tokens and control tokens are accepted. Token lookup checks
  `installation_credentials.control_token_hash` first, then falls through to
  `slots.token_hash` for node tokens (existing `LookupToken` path).
- `Cache-Control: no-store` MUST be set (the response confirms token validity).
- No rate limiting beyond standard connection limits. The endpoint reveals
  only whether a token is valid; the token itself is >256-bit random and
  non-guessable.

### 7. `POST /v1/recovery/rotate`

Auth: control token only (Bearer header).

**Request:**

```json
{}
```

**Success `200 OK`:**

```json
{
  "recovery_id": "rec_a1b2c3d4e5f6789001a2b3c4d5e6f789",
  "recovery_secret": "ABCDEFGHJKMNPQRSTVWXYZ23456",
  "shown_once": true
}
```

Rules:

- Node tokens MUST fail with `403 insufficient_capability`.
- Rotation MUST be an explicit user action. Create and recover flows MUST NOT
  silently call it.
- Revoked actors: `401 unauthorized` (token revoked).
- Left membership: `403 insufficient_capability`.
- Disabled orbit: `403 insufficient_capability`.
- Rate limit: 3 successful rotations per actor per rolling 60 minutes.

**Single-row overwrite model:**

Each `installation_credentials` row has exactly **one current recovery
generation**. There is no multi-row generation history table. Rotation
atomically overwrites the recovery state on the single row:

```sql
UPDATE installation_credentials
SET recovery_id = ?,
    recovery_secret_hash = ?,
    consumed_at = NULL
WHERE actor_id = ?
```

- `recovery_id` is replaced with a new 128-bit handle. The previous
  `recovery_id` becomes permanently invalid for lookup and consume.
- `recovery_secret_hash` is replaced with the SHA-256 hash of the new secret.
- `consumed_at` is reset to `NULL` (the new secret is unconsumed).
- `control_token_hash` is NOT modified. The current control token remains valid.
- Node token, membership, role, and actor identity are NOT modified.

**Rotation-vs-consume serialization:**

Rotation and consume both write to the same `installation_credentials` row.
SQLite's database-level writer serialization ensures only one proceeds at a
time. If a consume request is in-flight with the old `recovery_id` and a
rotation replaces it:

- The consume's generation-bound conditional write (§5.2 step 7c) predicates on
  `recovery_id = ?` (the old value). After rotation, the row's `recovery_id`
  has changed, so `RowsAffected() == 0`.
- The consume reloads (§5.2 step 8a), finds a different `recovery_id`, and
  returns `credential_invalid` at step 8c.
- The new generation's `control_token_hash` is never overwritten.

**Collision retry:** if the newly generated `recovery_id` collides with a
`UNIQUE` constraint (astronomically unlikely), generate-and-retry.

**Audit:** log the old `recovery_id` (non-secret handle) being replaced and the
new `recovery_id` being issued. No plaintext secret in audit.

**Idempotency:** rotation is NOT idempotent. Each call generates new material
with a new `recovery_id`. If the response is lost, the user may call rotate
again because the control token remains valid. The server MUST NOT keep
replayable plaintext copies.

**Idempotent replay lifetime of consumed secrets:** after rotation, any
idempotent replay of the old consumed secret (§5.2 step 8) fails because the
old `recovery_id` no longer exists on the row. This is safe because the client
already promoted the control token from the original consume.

### 8. Uniform Error Envelope

Content-Type: `application/json; charset=utf-8`

Every error response MUST use this exact body schema:

```json
{
  "error": {
    "code": "credential_invalid",
    "message": "The provided credential is not valid.",
    "retry_after_seconds": null
  }
}
```

| HTTP | `error.code` | `error.message` (stable, public) | When |
|---|---|---|---|
| `400` | `invalid_request` | `"The request is malformed or contains invalid parameters."` | Missing fields, invalid JSON, bad format, illegal `desired_role`, body too large, field length exceeded |
| `401` | `unauthorized` | `"Authentication is required."` | Missing, malformed, expired, or revoked Bearer token on authenticated endpoints (`/actor/context`, `/rotate`, `/telegram-links`) |
| `403` | `insufficient_capability` | `"This token does not have the required capability."` | Valid token, wrong capability (node token on control-only endpoint), or valid token but no active role/orbit authority (satellite actor, left membership, disabled orbit) |
| `403` | `credential_invalid` | `"The provided credential is not valid."` | Unknown, expired, used, revoked, race-loser — for unauthenticated secret consumes (recovery, device invite, Telegram link). ONE code for all secret failures. No timing or content difference. |
| `409` | `already_linked_same_orbit` | `"This Telegram account is already linked to this orbit."` | Telegram user has active membership in target orbit |
| `409` | `telegram_member_of_other_orbit` | `"This Telegram account belongs to a different orbit."` | Telegram user has active membership in another orbit (Phase 1: one orbit per Telegram user) |
| `429` | `too_many_attempts` | `"Too many attempts. Please wait before retrying."` | Rate limit exceeded |

**Distinction between `401 unauthorized`, `403 insufficient_capability`, and
`403 credential_invalid`:**

- `401 unauthorized` is for **authenticated endpoints** (Bearer token in header)
  when the token is missing, malformed, expired, or revoked. This follows
  standard HTTP semantics for bearer authentication.
- `403 insufficient_capability` is for **authenticated endpoints** where the
  token is valid but lacks the required capability or active context: node
  token on a control-only endpoint, satellite actor attempting issuance,
  left membership, or disabled orbit.
- `403 credential_invalid` is for **unauthenticated secret consumes** (recovery,
  device invite, Telegram link) where the secret itself is invalid. The uniform
  generic response prevents enumeration.

**`Retry-After` rules:**

- `429` responses MUST include `retry_after_seconds` as a positive integer
  (seconds until the next attempt is permitted). The value is also sent in
  the `Retry-After` HTTP header (integer seconds, per
  [RFC 9110 §10.2.3](https://www.rfc-editor.org/rfc/rfc9110#section-10.2.3)).
- All other error codes set `retry_after_seconds: null` and omit the header.

### 9. Brute-Force Controls

**Atomic attempt reservation:**

Every syntactically valid consume request atomically reserves (increments) an
attempt counter BEFORE any database lookup or hash verification. This prevents
a race where many concurrent requests all pass a pre-check window before any
failure is recorded. The counter counts ALL syntactically valid attempts (not
only failures) — this is simpler, concurrency-safe, and ensures each attempt
consumes a slot regardless of outcome.

**Limiter processing order (recovery consume):**

1. Input format validation → `400` (no counter touch).
2. Source-IP attempt counter: atomically increment → if counter exceeds limit,
   return `429`. The increment and check MUST be a single atomic operation.
3. Per-`recovery_id` attempt counter: atomically increment → if counter exceeds
   limit, return `429`.
4. Database lookup + SHA-256 comparison (or dummy comparison for unknown IDs).

**Rate limit table:**

| Endpoint | Key | Window | Limit | Counts |
|---|---|---|---|---|
| `POST /v1/recovery/consume` | source IP | 15 min rolling | 30 | All syntactically valid attempts |
| `POST /v1/recovery/consume` | `recovery_id` | 15 min rolling | 10 | All syntactically valid attempts |
| `POST /v1/recovery/rotate` | `actor_id` (from token) | 60 min rolling | 3 | Successful rotations only |
| `POST /v1/telegram-links` | `actor_id` (from token) | 60 min rolling | 5 | Successful issuances only |
| Telegram link consume | `telegram_user_id` (from Update) | 15 min rolling | 10 | All syntactically valid attempts |
| `POST /v1/device-invites/consume` | source IP | 15 min rolling | 20 | All syntactically valid attempts |

**Bounded limiter keys:**

- A syntactically valid fake `recovery_id` (matching `^rec_[0-9a-f]{32}$` but
  corresponding to no database row) CAN create an LRU limiter key. Format
  validation alone does not bound keys to database rows.
- The explicit **10,000-entry LRU cap** per endpoint bounds total limiter state.
  An entry expires when its window closes. Entries are evicted LRU when the cap
  is reached. This limits an attacker's ability to exhaust limiter memory with
  arbitrary syntactically valid IDs.
- The source-IP limiter applies to every syntactically valid attempt before any
  per-`recovery_id` counter or hash work — including unknown IDs.
- Format-invalid inputs (`400 invalid_request`) never touch the limiter.

**No materially different fast path:**

- The `credential_invalid` code path MUST NOT short-circuit before hash
  comparison for "not found" versus "found but wrong". Use constant-time
  SHA-256 comparison against `dummy_hash` for non-existent IDs and for
  revoked/left/disabled states. Total response time for unknown IDs, wrong
  secrets, expired codes, consumed codes, and disabled contexts MUST be
  statistically indistinguishable.

**Phase 1 limitation — process restart:**

Limiter state is in-memory only. It resets on coordinator process restart.
This is accepted for Phase 1. A persistent limiter (e.g., SQLite-backed
sliding window) is a documented future extension.

### 10. `POST /v1/telegram-links`

Auth: control token only (Bearer header).

**Request:**

```json
{
  "desired_role": "companion"
}
```

`desired_role` is optional. Allowed values: `companion`, `satellite`. Omission
defaults to `companion`. `primary` MUST fail with `400 invalid_request`;
primary role transfer stays on the existing `/make_primary` path and is never
granted via Telegram link.

**Success `201 Created`:**

```json
{
  "link_code": "ABCDEFGHJKMNPQRSTVWXYZ23456",
  "desired_role": "companion",
  "expires_at": "2026-07-12T16:45:00Z",
  "bot_username": "barycenter_bot"
}
```

The response includes the bot username for display only. It MUST NOT include a
URL with the link code embedded (no `?start=CODE` deep link).

**Authorization matrix for Telegram link issuance:**

| Issuer context | `companion` link | `satellite` link |
|---|---|---|
| Active `primary` with valid control token | Allowed | Allowed |
| Active `companion` with valid control token | Allowed | Allowed |
| `satellite` actor | `403 insufficient_capability` | `403 insufficient_capability` |
| Revoked actor | `401 unauthorized` (token revoked) | `401 unauthorized` |
| Left membership | `403 insufficient_capability` | `403 insufficient_capability` |
| Disabled orbit | `403 insufficient_capability` | `403 insufficient_capability` |

Rationale: both `primary` and `companion` may issue either `companion` or
`satellite` links. This keeps Telegram linking usable for companion-owned
installations while capping grantable authority below `primary` (which is never
grantable via link). Satellite actors have no issuance capability. Disabled
orbits and left memberships deny issuance because the actor lacks active orbit
authority, not because the token is invalid.

Rules:

- `link_code` uses `generateSecret(27)` (132.49-bit, rejection-sampled).
- TTL is exactly 15 minutes from issue time.
- Issuing a new link code for the same actor revokes that actor's older
  unconsumed Telegram link codes immediately by setting `invalidated_at`:
  ```sql
  UPDATE telegram_link_codes
  SET invalidated_at = ?
  WHERE issuer_actor_id = ?
    AND consumed_at IS NULL
    AND invalidated_at IS NULL
  ```
  This ensures the consume predicate (§11) rejects stale codes.
- The `issuer_actor_id` on the `TelegramLinkCode` row is the issuing app actor.
  It is not an ownership transfer to Telegram.
- Input: `desired_role` max 12 characters; request body max 256 bytes.

### 11. Telegram Link Consume (In-Process Service Method)

**Phase 1 trust boundary:** Telegram link consume is an **in-process service
method** on the coordinator, not a public or internal HTTP endpoint. The
coordinator process receives Telegram updates via authenticated TLS long
polling using the protected bot token. The `telegram_user_id` is derived
from the verified `Update.message.from.id` field by the bot handler code
running in the same process.

Trust comes from the authenticated Bot API transport (TLS connection to
api.telegram.org) and the secrecy of the bot token. The `Update` object
itself carries no cryptographic signature in long-polling mode. An in-process
consumer of these updates trusts `from.id` because only the authenticated
transport can deliver them.

A future architectural split (separate Telegram adapter process) would require
its own authenticated inter-service protocol decision. That decision is out of
scope for Phase 1 and MUST NOT be pre-empted by leaving a vague "service
credential" option in this contract.

Node tokens and control tokens cannot invoke Telegram link consume. They
operate in a different authentication domain.

**Service method signature (logical):**

```go
func (s *Service) ConsumeTelegramLink(
    telegramUserID  int64,   // from verified Update.message.from.id
    displayName     string,  // from Update.message.from (refreshable hint)
    chatType        string,  // from Update.message.chat.type
    linkCode        string,  // from message text, stripped/normalized
) (result ConsumeLinkResult, err error)
```

**Success result:**

```go
type ConsumeLinkResult struct {
    OrbitID  int64   // existing orbits.id
    ActorID  int64   // new or reactivated actors.id
    Role     string  // from link code's desired_role
}
```

**Rules:**

- `chatType` MUST be `"private"`. Group or channel contexts MUST be rejected
  (the bot handler validates this from the Update before calling the service
  method).
- The bot handler derives `telegramUserID` from the verified Update. The
  service method trusts this value because it runs in the same process as the
  authenticated Bot API consumer.
- Membership is keyed on `telegram_user_id`. `displayName` is a refreshable
  display hint, updated on each interaction.
- `desired_role` is applied only on first successful membership creation. It
  never upgrades, downgrades, or transfers a migrated member's role.

**Rollback-safe transaction:**

The entire consume operation runs in one SQLite transaction (`BEGIN IMMEDIATE`).
Any failure at any step causes a `ROLLBACK`, which restores the code to its
unconsumed state — fulfilling the "code NOT consumed" promise for conflict and
error paths.

Processing order (all inside the transaction):

1. **Normalize and validate** `linkCode`. Strip whitespace/hyphens, uppercase.
   Validate against `^[ABCDEFGHJKMNPQRSTVWXYZ2-9]{27}$`. Invalid input: return
   `credential_invalid` (no transaction needed).
2. **Rate limit** by `telegram_user_id` (atomic reservation before the
   transaction, same as §9).
3. **`BEGIN IMMEDIATE`** — acquire SQLite write lock.
4. **Lookup code:**
   ```sql
   SELECT issuer_actor_id, orbit_id, desired_role, expires_at
   FROM telegram_link_codes
   WHERE code_hash = ?
     AND consumed_at IS NULL
     AND invalidated_at IS NULL
     AND expires_at > ?
   ```
   The predicate includes `invalidated_at IS NULL` (reissue revocation, §10)
   and `expires_at > ?` (TTL). If not found: `ROLLBACK`, compute
   `hashToken(submitted_code)` vs `dummy_hash` (timing equalization), return
   `credential_invalid`.
5. **Hash-verify the code:** compute `hashToken(normalized_code)` and
   constant-time compare against `code_hash`. If mismatch: `ROLLBACK`, return
   `credential_invalid`.
6. **Resolve or create the Telegram actor:**
   - Query `actors` for `kind = 'telegram_user'` and
     `external_ref = '{telegram_user_id}'`.
   - If found and `revoked_at IS NOT NULL`: `ROLLBACK`, return
     `credential_invalid`. Revocation is stronger than re-linking.
   - If not found: `INSERT INTO actors (kind, external_ref, display_name, created_at)
     VALUES ('telegram_user', ?, ?, ?)`. Use the returned `actor_id`.
   - If found and not revoked: reuse the existing `actor_id`. Update
     `display_name` if changed.
7. **Check existing membership:**
   ```sql
   SELECT orbit_id, role FROM memberships
   WHERE actor_id = ? AND left_at IS NULL
   ```
   - If active membership in the **same orbit** as the code's `orbit_id`:
     `ROLLBACK`, return `already_linked_same_orbit`. Code NOT consumed.
   - If active membership in a **different orbit** (Phase 1: one orbit per
     Telegram user): `ROLLBACK`, return `telegram_member_of_other_orbit`.
     Code NOT consumed.
8. **Reserve the code (conditional write):**
   ```sql
   UPDATE telegram_link_codes
   SET consumed_at = ?, consuming_actor_id = ?
   WHERE code_hash = ?
     AND consumed_at IS NULL
     AND invalidated_at IS NULL
     AND expires_at > ?
   ```
   Check `RowsAffected()`: if `rows == 0`, a concurrent consumer won the race
   (or the code expired/was invalidated between step 4 and step 8).
   `ROLLBACK`, return `credential_invalid`.
9. **Create or reactivate membership:**
   - If the actor had a previous membership in this orbit with
     `left_at IS NOT NULL`: reactivate with `left_at = NULL`,
     `joined_at = now`, at the code's `desired_role`. This is a new membership
     grant, not escalation.
   - Otherwise: `INSERT INTO memberships (orbit_id, actor_id, role, joined_at)
     VALUES (?, ?, ?, ?)`.
   - If the INSERT fails (e.g., partial unique index violation from a race):
     `ROLLBACK`, return `credential_invalid`. The code is restored to
     unconsumed.
10. **Dual-write legacy `members` table** (§17 coexistence):
    ```sql
    INSERT OR REPLACE INTO members (orbit_id, tg_user_id, role, joined_at, display_name)
    VALUES (?, ?, ?, ?, ?)
    ```
    This keeps the legacy coordinator's view consistent.
11. **Audit** the consume event (no plaintext code in payload).
12. **`COMMIT`** — all-or-nothing. Return success with `{orbit_id, actor_id, role}`.

**Single-winner guarantee:** the conditional write at step 8 includes
`consumed_at IS NULL AND invalidated_at IS NULL AND expires_at > ?`. SQLite's
database-level writer serialization ensures only one concurrent consumer can set
`consumed_at`. The loser's `RowsAffected() == 0` triggers `ROLLBACK` — the
loser's actor creation and membership are also rolled back, preventing orphaned
state.

**Two-code/same-user race:** two different link codes consumed concurrently for
the same Telegram user both enter step 3. The first to reach step 9 inserts the
membership; the second hits the partial unique index
`ON memberships(actor_id) WHERE left_at IS NULL` (§17) and fails, causing
`ROLLBACK`. The second code is restored to unconsumed, and the user can retry
if needed.

**Bot message hygiene:**

- The bot MUST NOT echo the link code back to the user in any message.
- On successful consume, the bot SHOULD best-effort delete the user's message
  containing the code (using Telegram `deleteMessage` API where permitted by
  chat permissions). Failure to delete is not an error.
- Error messages from the bot use the same generic language as the JSON error
  envelope — no information about why the code failed.

### 12. Concurrent Consume and Replay

- **Recovery:** exactly one transaction may consume a given
  `(recovery_id, recovery_secret)` pair via the generation-bound conditional
  write (`UPDATE ... WHERE recovery_id = ? AND consumed_at IS NULL` +
  `RowsAffected()` check). Concurrent losers reload
  current state (step 8a) before any comparison. Same-tuple replay returns
  idempotent success (step 8h) — but only until the next rotation replaces the
  `recovery_id`. A concurrent rotation causes the old consume to fail at step
  8c. Different-tuple replay or different replacement token returns
  `403 credential_invalid` (step 8i).
- **Telegram link:** exactly one transaction may consume a given `link_code`
  via the rollback-safe transaction (§11). The conditional code write includes
  `consumed_at IS NULL AND invalidated_at IS NULL AND expires_at > ?`.
  Concurrent losers trigger `ROLLBACK` — actor creation and membership are
  also rolled back. Code is restored to unconsumed only on failure paths; on
  success it is permanently consumed. Post-success replays and concurrent losers
  return `credential_invalid`.
- **Device invite:** same winner-takes-code pattern as the existing
  `UPDATE ... WHERE used_at IS NULL` at
  `coordinator/internal/store/orbits.go:536-541`.
- All successful consumes and all `429 too_many_attempts` events MUST be
  audited without plaintext secret material in the audit payload.

### 13. Secret Hygiene and Redaction

Recovery secrets, Telegram link codes, device invite codes, bearer tokens, and
any plain bot message that carries them MUST be redacted from:

- Server logs (structured and unstructured);
- Audit event payloads (store only hashes or non-secret IDs);
- Analytics and product telemetry;
- Crash reports and error dialogs;
- URLs, path segments, and query strings;
- Telegram message text that the bot sends (never echo secrets; show only
  non-secret identifiers or confirmation text).

The app may expose explicit copy/save actions for recovery material, but MUST
NOT silently persist secrets. A plain bot username (`@barycenter_bot`) without
the secret is allowed. A deep link or URL whose parameter contains a secret is
forbidden.

### 14. HTTP and Credential Hygiene

- **HTTPS required** for all secret-bearing endpoints outside loopback
  (`127.0.0.1` / `::1`) test environments.
- **`Cache-Control: no-store`** MUST be set on every HTTP response that
  contains or accepts secret material: `/v1/onboarding/orbits` (creation
  response), `/v1/recovery/consume` (request + response),
  `/v1/recovery/rotate` (response), `/v1/telegram-links` (response),
  `/v1/device-invites` (response), `/v1/device-invites/consume` (request),
  `/v1/actor/context` (response — confirms token validity).
- Request bodies on these paths MUST be excluded from access logs and error
  logs. If the web framework logs request bodies by default, these routes
  must be explicitly exempted.
- Invalid or revoked Bearer tokens on authenticated endpoints produce
  `401 unauthorized` (standard HTTP bearer semantics), not
  `403 credential_invalid`.
- `403 credential_invalid` is reserved exclusively for unauthenticated
  secret-consume endpoints where the secret itself failed.
- `403 insufficient_capability` is used on authenticated endpoints where the
  token is valid but lacks the required capability or active context.

### 15. Post-Recovery Credential State

| Credential | After successful recovery |
|---|---|
| Control token | **Replaced** by `replacement_control_token` from request. The previous control token hash is overwritten on the `installation_credentials` row. Server stores only the SHA-256 hash of the token string (64-char hex → `hashToken()` → 64-char hex digest). This overwrite is the sole revocation mechanism; there is no control token history table in Phase 1. |
| Node token | **Preserved** in `slots.token_hash`. If the installation still has it locally, it keeps working. If lost, the installation is in control-only state until rebind. The `installation_credentials` row references the slot but does not duplicate or modify the node token hash. |
| Recovery secret | **Consumed** (marked used). No new secret is issued automatically. User may call `/v1/recovery/rotate` explicitly. |
| Membership | **Unchanged**. Role, orbit, actor identity remain as they were. |

### 16. Schema Ownership

**Separation of concerns:**

- **`actors`** — identity only, no secrets. Columns: `id INTEGER PRIMARY KEY`,
  `kind TEXT` (`'app_installation'` | `'telegram_user'`),
  `display_name TEXT`, `external_ref TEXT`, `created_at INTEGER`,
  `revoked_at INTEGER`.
- **`memberships`** — role binding. Columns: `orbit_id INTEGER`,
  `actor_id INTEGER`, `role TEXT`, `joined_at INTEGER`, `left_at INTEGER`.
  Primary key: `(orbit_id, actor_id)`.
- **`installation_credentials`** — secrets for `app_installation` actors only.
  Columns: `actor_id INTEGER PRIMARY KEY` (references `actors.id`),
  `slot_orbit_id INTEGER`, `slot_name TEXT` (together reference
  `slots(orbit_id, slot)` for the authoritative node token hash),
  `control_token_hash TEXT` (64-char hex, SHA-256 of token string bytes),
  `recovery_id TEXT UNIQUE`,
  `recovery_secret_hash TEXT` (64-char hex, SHA-256 of normalized secret),
  `consumed_at INTEGER`, `created_at INTEGER`.
  The single-row model (§7) means rotation overwrites `recovery_id`,
  `recovery_secret_hash`, and resets `consumed_at`. There is no
  `invalidated_at` column — rotation replaces the row's recovery state rather
  than marking it. Admin invalidation without replacement revokes the actor
  (`actors.revoked_at`) which fails all recovery and authentication.
- **`telegram_link_codes`** — link codes. Columns: `code_hash TEXT` (64-char
  hex, SHA-256 of normalized code), `issuer_actor_id INTEGER`,
  `orbit_id INTEGER`, `desired_role TEXT DEFAULT 'companion'`,
  `expires_at INTEGER`, `invalidated_at INTEGER` (set when a newer code is
  issued for the same actor, §10), `consumed_at INTEGER`,
  `consuming_actor_id INTEGER`, `created_at INTEGER`.
- **`slots`** — existing table, unchanged. `token_hash TEXT` remains the
  authoritative store for node token hashes (unkeyed SHA-256 of token string
  bytes, hex-encoded, 64 characters). `installation_credentials` references the
  slot; it does not duplicate `token_hash`.

**Revocation mechanism (Phase 1):** replacing the current `control_token_hash`
on the `installation_credentials` row is the sole control-token revocation
mechanism. The previous hash is overwritten; no history record is kept. Admin
revocation of the actor itself (`actors.revoked_at`) makes ALL authentication
fail regardless of token validity. No control token version/history table
exists in Phase 1.

### 17. Additive Schema / Legacy Coexistence

**Core rule:** existing `members` and `slots` tables remain intact and
unmodified by the migration. Backfill is idempotent and never deletes or
rewrites their role or token rows. Rollback to the previous coordinator with
the feature flag off is safe — the old code reads `members`/`slots` as before
and ignores the additive tables.

**Credential provisioning for backfilled installations:**

Legacy slots cannot receive newly minted control tokens or recovery secrets
during backfill because there is no plaintext delivery channel — the node token
was delivered at pair time and its plaintext was never stored. Therefore, for
backfilled `installation_credentials` rows:
- `control_token_hash` is `NULL` (unprovisioned until the installation upgrades
  and explicitly obtains a control token through a separately authorized flow).
- `recovery_id` is `NULL` and `recovery_secret_hash` is `NULL` (unprovisioned
  until the installation receives its first recovery material at provisioning
  time).
- The `(slot_orbit_id, slot_name)` reference to `slots` is populated and the
  node token hash remains authoritative in `slots.token_hash`.

An unprovisioned `installation_credentials` row signals "legacy installation
not yet upgraded." Authentication falls through to the existing
`LookupToken` path via `slots.token_hash` for playback/heartbeat/media only.

**No node-token control escalation:**

A node token grants playback, heartbeat, and scoped media download only. It
MUST NOT be used to provision control tokens, recovery material, or any
administrative capability. This is a core invariant of the node/control
capability split (spec §6.1).

A legacy unprovisioned installation MUST obtain control authority through a
**separately authorized flow**:
- **Device invite from primary/companion:** an existing primary or companion
  actor issues a device invite (§spec 11.1 `/v1/device-invites`). The
  installation consumes the invite, which provisions its control credential.
  The installation may simultaneously prove slot possession (node token) as
  additional evidence that it owns the expected slot, but node possession alone
  is insufficient.
- **Explicit Telegram-owner authorization:** the orbit's Telegram owner
  confirms the upgrade via an interactive bot flow (e.g., in response to a
  `/upgrade` or `/provision` command that identifies the slot). This leverages
  the existing Telegram-based trust model.
- **Another frozen separately-authorized proof:** any mechanism that provides
  independent authorization from a principal with sufficient capability
  (primary or companion), without relying solely on node-token authentication.

The backfilled role (derived from `paired_by`) is informational until control
provisioning is complete. A backfilled actor with `role = 'primary'` or
`role = 'companion'` does NOT have usable control authority until the separately
authorized provisioning issues a control token and recovery material.

**Negative test requirement:** a test MUST verify that node-token-only
authentication cannot provision control/recovery credentials. This applies to
all API endpoints and onboarding flows.

**Uniqueness constraints preventing identity drift:**

| Constraint | Mechanism | Violation handling |
|---|---|---|
| One Telegram actor per external Telegram ID | `UNIQUE INDEX ON actors(kind, external_ref) WHERE kind = 'telegram_user'` | Reuse existing actor (reactivate if left) |
| One installation actor per slot reference | `UNIQUE(slot_orbit_id, slot_name) ON installation_credentials` | Reject duplicate; each slot maps to exactly one actor |
| Unique recovery lookup handle | `UNIQUE(recovery_id) ON installation_credentials WHERE recovery_id IS NOT NULL` | Generate-and-retry on collision (astronomically unlikely) |
| Unique code lookup hash | `UNIQUE INDEX ON telegram_link_codes(code_hash)` | Generate-and-retry on collision |
| At most one active membership per actor (Phase 1) | `CREATE UNIQUE INDEX memberships_one_active ON memberships(actor_id) WHERE left_at IS NULL` | Database-enforced; INSERT fails. Telegram consume: ROLLBACK restores code; return `already_linked_same_orbit` or `telegram_member_of_other_orbit` depending on orbit. Recovery: N/A (recovery does not create memberships). |
| One orbit per Telegram user (Phase 1) | Same partial unique index `memberships_one_active` (since Phase 1 has one membership per actor, and each Telegram user maps to one actor, this is sufficient) | Same rejection as above |

**Database-enforced uniqueness (not application-only):** the `memberships_one_active`
partial unique index replaces the prior application-level check. An
application-level check alone is race-prone: two concurrent transactions can
both pass the check and both insert. The database index makes the INSERT itself
fail atomically, ensuring the ROLLBACK in §11 restores the code to unconsumed.

**Deriving installation actor and role from `paired_by`:**

During backfill, each `slots` row is mapped to an `app_installation` actor:
- `actors.kind = 'app_installation'`
- `actors.external_ref = "{orbit_id}:{slot}"` (stable, unique per slot)
- `actors.display_name = slot` (e.g., `"A"`, `"B"`)
- Membership role is derived from `slots.paired_by`:
  - If `paired_by` matches the orbit creator's `tg_user_id` (the member with
    `role = 'primary'` in the existing `members` table): the installation actor
    gets `role = 'primary'` in `memberships`.
  - Otherwise (consistent `paired_by` matching a non-primary member): the
    installation actor gets `role = 'companion'`.
  - **Orphan/inconsistent rows** (slot with `paired_by = 0` or `paired_by`
    referencing no existing member): the installation actor gets
    `role = 'satellite'` in `memberships`. Satellite is the lowest-capability
    role: it cannot issue links, exercise control, or grant memberships. This
    prevents an orphan from inheriting companion authority (which includes link
    issuance) by default. A warning is logged during backfill. Explicit
    authorized repair (via primary/companion action) is required to upgrade.
- `installation_credentials` references the slot via `(slot_orbit_id, slot_name)`.
- **Backfilled role does not confer usable control authority** until the
  separately authorized provisioning (above) completes. A backfilled
  `primary` or `companion` with `control_token_hash = NULL` can authenticate
  only via the node token's `LookupToken` path for playback, not for
  control/admin operations.
- Backfill is idempotent: if the actor/membership/credential row already exists
  with matching values, no mutation occurs. If it exists with different values,
  backfill logs a warning and does NOT overwrite (prevents drift on re-run).

**Legacy `members` → Telegram actors:**

Each `members` row is mapped to a `telegram_user` actor:
- `actors.kind = 'telegram_user'`
- `actors.external_ref = "{tg_user_id}"` (string of the integer)
- `actors.display_name = members.display_name`
- Membership role = `members.role`
- No `installation_credentials` row (Telegram actors have no app credentials)
- Same idempotency and no-overwrite rules as above.

**Dual-write coexistence for Telegram memberships:**

New or reactivated Telegram memberships created via link consume (§11)
MUST transactionally keep the legacy `members` table consistent. This ensures
the previous coordinator (feature-flag-off) sees the same role and membership:

- **On successful Telegram link consume (§11 step 10):**
  ```sql
  INSERT OR REPLACE INTO members (orbit_id, tg_user_id, role, joined_at, display_name)
  VALUES (?, ?, ?, ?, ?)
  ```
  This is inside the same transaction as the membership creation.

- **On membership leave** (when a Telegram user's membership is set to
  `left_at IS NOT NULL`):
  ```sql
  DELETE FROM members WHERE orbit_id = ? AND tg_user_id = ?
  ```

- **On membership role change** (if ever applicable in Phase 1):
  ```sql
  UPDATE members SET role = ? WHERE orbit_id = ? AND tg_user_id = ?
  ```

The legacy `members_user` unique index (`UNIQUE INDEX ON members(tg_user_id)`)
enforces the same one-orbit-per-user constraint for the legacy view. This is
consistent with the new `memberships_one_active` partial unique index.

Feature-flag-off behavior: the old coordinator reads `members` and `slots`
unchanged. The additive tables (`actors`, `memberships`,
`installation_credentials`, `telegram_link_codes`) are ignored. No mutation
to legacy tables occurs except through the dual-write paths above, which
produce the same data the old coordinator would have created directly.

---

## Downstream Task Impact

- **Schema task** (`TASK-260712-1bpog0`): add `actors` table (identity only,
  no secret columns). Add `memberships` table with partial unique index
  `memberships_one_active ON memberships(actor_id) WHERE left_at IS NULL`
  (database-enforced, not application-only). Add
  `installation_credentials` table with `actor_id INTEGER PRIMARY KEY`,
  `slot_orbit_id`, `slot_name`, `control_token_hash TEXT` (nullable for
  backfill), `recovery_id TEXT UNIQUE` (nullable), `recovery_secret_hash TEXT`
  (nullable), `consumed_at`, `created_at`. No `invalidated_at` column —
  single-row rotation model (§7) overwrites recovery state directly. All hash
  columns are 64-char lowercase hex TEXT matching `slots.token_hash`. Node token
  hash stays in `slots.token_hash`; the credential row references the slot via
  `(slot_orbit_id, slot_name)`. Add `telegram_link_codes` table with
  `code_hash TEXT` (64-char hex) and `invalidated_at INTEGER` (for reissue
  revocation, §10). Use `generateSecret(27)`. Use `hashToken()` (unkeyed
  SHA-256 of string bytes, hex-encoded). No HMAC key file. Add uniqueness
  constraints per §17 — all database-enforced partial unique indexes. Add
  idempotent backfill from `members`/`slots` with role derivation from
  `paired_by`: orphan/inconsistent rows get `satellite` (not `companion`).
  Backfill leaves `control_token_hash`/`recovery_id`/`recovery_secret_hash`
  as NULL. Legacy tables remain intact; feature-flag-off rollback is safe.
  Add test that reproduces the hash test vectors from §3.
- **API task** (`TASK-260712-m5264f`): implement `/v1/recovery/consume` with
  the **generation-bound** conditional-write single-winner transaction (§5.2):
  predicate on `recovery_id = ? AND consumed_at IS NULL`, NOT merely
  `actor_id`. On `RowsAffected() == 0`, **reload** current row before any
  idempotency comparison — never compare against stale pre-rotation fields.
  Atomic attempt reservation before hash work, idempotent-retry path with
  same-tuple check (reloaded data), and lifecycle-state rejection
  (revoked/left/disabled) on both first consume and idempotent replay.
  Implement `/v1/recovery/rotate` with single-row overwrite model (§7):
  atomically overwrite `recovery_id`, `recovery_secret_hash`, reset
  `consumed_at = NULL`. Implement `/v1/actor/context` (read-only probe with
  full response table: 200, 401, 403), `/v1/telegram-links` with the
  authorization matrix (disabled orbit → `403 insufficient_capability`, not
  `credential_invalid`) and `invalidated_at` on reissue. Implement
  `/v1/device-invites/consume`. Use the uniform error envelope with the
  three-way distinction: `401` for bearer, `403 insufficient_capability` for
  authority, `403 credential_invalid` for unauthenticated secrets. Use
  `hashToken()` consistently — hash the string bytes, not hex-decoded bytes.
  Reproduce the test vectors. Set `Cache-Control: no-store`. Exclude request
  bodies from logs. Dummy-hash verification for unknown IDs and
  revoked/left/disabled contexts. Source-IP limiter applies before per-ID
  limiter; both apply before hash work. Node tokens MUST NOT provision
  control/recovery (negative test required).
- **Telegram adapter task** (`TASK-260712-2xkyot`): consume is an in-process
  service method, not an HTTP endpoint. Derive `telegram_user_id` from
  verified `Update.message.from.id`. Validate `chat_type == "private"`. Never
  echo link codes. Best-effort delete consumed code messages. Use
  **rollback-safe transaction** (§11): `BEGIN IMMEDIATE`, resolve/create actor,
  check conflicts, conditional code write (`WHERE consumed_at IS NULL AND
  invalidated_at IS NULL AND expires_at > ?` + `RowsAffected()`),
  create/reactivate membership, **dual-write legacy `members` table** (§17),
  audit, `COMMIT`. On any failure: `ROLLBACK` restores code to unconsumed.
- **Client tasks** (`TASK-260712-2u1w16`, `TASK-260712-47uve0`): implement
  the pending-credential protocol (§5.1) with `ever_sent` marker. Generate
  `replacement_control_token` (32 bytes, hex) and write to Keychain/DPAPI
  before sending. Set `ever_sent = true` before network call. NEVER auto-delete
  a pending credential once `ever_sent` is true. Handle probe per the complete
  response table (§5.1.1): 200 → promote, 403 → promote (limited context),
  401 → retry recovery with same tuple, 5xx/network → retry with backoff.
  On `403` from recovery: do NOT delete pending. On `401` from probe followed
  by `403` from recovery retry: do NOT delete pending (race note applies).
  Destructive-abandon Cancel requires explicit user confirmation with loss
  warning. Show recovery material once. Never build secret-bearing URLs.
  Use the exact alphabet regex for input validation.
- **Test task** (`TASK-260712-38qsku`): hash test vector reproduction
  (`hashToken("000...0")` = `60e05bd...`; `hashToken("ABCDEFG...")` =
  `e45d609...`). Legacy `LookupToken` compatibility: generate token with
  `randomHex(32)`, store `hashToken(token)`, authenticate with same string.
  **Consume-vs-rotate race:** concurrent rotate during inflight consume must
  cause consume to fail (old `recovery_id` no longer matches) and never
  overwrite the new generation's `control_token_hash`. **Consume-vs-revoke/
  leave/disable:** consume must fail for subsequently revoked/left/disabled
  actors, including idempotent replay path. **Two-token concurrency:** two
  concurrent consume requests with different `replacement_control_token` values
  — exactly one wins, the other gets `credential_invalid`. **Idempotency
  lifetime after rotation:** consumed secret's replay succeeds before rotation,
  fails after rotation replaces `recovery_id`. **Reload-on-zero-rows:**
  after `RowsAffected() == 0`, the code must reload current state and detect
  rotation vs concurrent consume vs admin invalidation, never compare stale
  data. Replay, concurrency (atomic attempt reservation under parallel
  requests, conditional-write winner/loser), rate-limit, timing
  indistinguishability (unknown vs wrong vs consumed vs disabled), redaction,
  control-only recovery, role-preservation, idempotent-retry (same tuple:
  success; different token: fail), pending-credential crash-and-retry (probe
  200 → promote; probe 401 → retry recovery; probe 403 → promote limited;
  probe network-error → retry with no pending delete), `ever_sent` state
  machine (never auto-delete once sent), revoked/left/disabled rejection with
  correct error codes on both first consume and idempotent replay,
  authorization matrix edge cases (disabled orbit → `insufficient_capability`),
  exact alphabet validation (reject `I`, `L`, `O`, `U`), dummy-hash path, bot
  message deletion, `Cache-Control: no-store` header presence, actor-context
  probe endpoint with valid/invalid/revoked tokens and left/disabled membership
  responses, backfill idempotency and role derivation from `paired_by` (orphan
  rows → `satellite`). **Telegram rollback-safe transaction tests:**
  same-code/two-user (exactly one winner, code restored for loser),
  two-code/same-user (second code fails at membership partial unique index,
  code restored), expiry-boundary (code expired between lookup and conditional
  write), reissue-vs-consume (new issuance invalidates old code via
  `invalidated_at`, concurrent consume of old code fails),
  membership-insert-failure (constraint violation rolls back code consumption),
  dual-write consistency (legacy `members` row matches new `memberships` row
  after consume). **Negative node-token escalation test:** verify that
  node-token-only authentication cannot provision control/recovery credentials
  on any endpoint or flow.

---

## Answers to Task Questions

| Question | Answer |
|---|---|
| Recovery endpoint path | `POST /v1/recovery/consume` |
| Recovery request shape | `{recovery_id, recovery_secret, replacement_control_token}` |
| Recovery response shape | `{orbit_id, actor_id, role}` — flat, single membership |
| Uniform errors | §8: `credential_invalid` for unauthenticated secret failures; `unauthorized` for bearer token failures; `insufficient_capability` for valid token lacking authority (satellite, left, disabled orbit) |
| Rate limits | §9: per-IP (30/15min), per-recovery_id (10/15min), per-actor rotation (3/60min), per-actor link issuance (5/60min), per-telegram_user consume (10/15min) — all syntactically valid attempts counted atomically before hash work |
| Secret rotation | Explicit `POST /v1/recovery/rotate` with control token auth |
| Control-token revocation | Recovery overwrites the single `control_token_hash` on `installation_credentials`; no history table. Admin revocation via `actors.revoked_at`. |
| One-time display | Recovery secret shown only at create and explicit rotate |
| Nonpersistence | Client MUST NOT silently persist the secret alongside credentials |
| Post-recovery credential state | Control-only reissue via client-supplied token; node preserved in `slots.token_hash` |
| Telegram desired-role | `companion` (default) or `satellite`; `primary` is `400 invalid_request` |
| Code entropy | 27 chars × 30-symbol alphabet via rejection sampling = 132.49 bits |
| Code expiry | Recovery: single-use, no TTL. Telegram link: 15 minutes |
| Same-orbit conflict | `409 already_linked_same_orbit`; code NOT consumed |
| Foreign-orbit conflict | `409 telegram_member_of_other_orbit`; code NOT consumed |
| Already-linked | Same as same-orbit conflict |
| Concurrent consume | Recovery: exactly one winner via generation-bound conditional write (`recovery_id` + `consumed_at IS NULL`) + RowsAffected(); losers reload current state before idempotency check. Telegram: rollback-safe transaction (§11); loser's ROLLBACK restores code. |
| Lost response | Client probes `GET /v1/actor/context` with pending token; if 200, promotes directly; if 403, also promotes (token valid, context limited); if 401, retries recovery with same tuple |
| Client crash | `ever_sent` marker in Keychain/DPAPI; pending token NEVER auto-deleted once sent; probe endpoint determines validity; any non-success does not delete pending state |
| Telegram caller trust | In-process service method; principal from verified Update via authenticated Bot API transport (TLS long polling + bot token) |
| Unrecoverable loss | Sole installation + unsaved secret = unrecoverable (stated in UI) |
| Revoked/left/disabled | Recovery and link consume fail with generic `credential_invalid` (dummy hash comparison for timing); authenticated endpoints fail with `401` (revoked token) or `403 insufficient_capability` (valid token, no authority). Applies to both first consume and idempotent replay. |
| Authorization matrix | Primary or companion: both link roles. Satellite/revoked/left/disabled: none. Primary never granted via link. Disabled orbit on authenticated endpoints: `403 insufficient_capability`. |
| At-rest hash | Unkeyed SHA-256 of canonical string bytes (not hex-decoded), matching existing `hashToken()`. Hex TEXT storage. Test vectors provided. No HMAC key. |
| Identifier types | `orbit_id`: integer. `actor_id`: integer. `recovery_id`: `rec_` + 32 hex. `telegram_user_id`: string. |
| HTTP hygiene | HTTPS required. `Cache-Control: no-store`. Bodies excluded from logs. |
| Schema ownership | `actors` = identity (no secrets). `installation_credentials` = control/recovery hashes + slot reference. `slots` = authoritative node token hash (unchanged). |
| Actor context probe | `GET /v1/actor/context` — read-only, accepts node or control token, returns `{orbit_id, actor_id, role}` or `401` or `403 insufficient_capability`. Full probe response table in §5.1.1. |
| Cancel pending recovery | Requires destructive-abandon confirmation if `ever_sent` is true and probe has not confirmed 401 + recovery retry also returned 403 |
| Pending state `ever_sent` | Once true, pending credential is never auto-deleted. Terminal conditions: (a) promoted, (b) superseded by confirmed credential, (c) user-confirmed destructive abandon. |
| Hash input convention | SHA-256 of the token/secret's canonical string bytes (64 ASCII hex chars for tokens, 27 uppercase chars for secrets). NOT hex-decoded binary. |
| Legacy coexistence | `members` and `slots` tables remain intact. Backfill is idempotent, never deletes/rewrites. Credential columns nullable for unprovisioned installations. Feature-flag-off rollback is safe. Telegram membership mutations dual-write to legacy `members` table (§17). |
| Role derivation from `paired_by` | Matches orbit creator's `tg_user_id` → primary. Otherwise → companion. Orphan/inconsistent rows → `satellite` (lowest capability, cannot issue links). Explicit authorized repair required to upgrade. |
| Single-winner mechanism | Recovery: generation-bound `UPDATE ... WHERE recovery_id = ? AND consumed_at IS NULL` + `RowsAffected()` → reload on zero rows. Telegram: rollback-safe transaction with `UPDATE ... WHERE consumed_at IS NULL AND invalidated_at IS NULL AND expires_at > ?` + `RowsAffected()`. Same conditional-write principle as existing invite consume at line 537. |
| Node-token escalation | Forbidden. Node token grants playback/heartbeat/media only. Legacy installations obtain control through a separately authorized flow (device invite, Telegram-owner authorization). |
| Recovery rotation model | Single-row overwrite: rotation atomically replaces `recovery_id`, `recovery_secret_hash`, resets `consumed_at`. No multi-row generation history. Old `recovery_id` permanently invalid. |
| Consume-vs-rotate | Concurrent rotation causes consume's conditional write to match zero rows (old `recovery_id` gone). Reload detects different `recovery_id` → `credential_invalid`. New generation's `control_token_hash` never overwritten. |
| Telegram transaction safety | Rollback-safe: actor creation, code reservation, membership creation, and legacy dual-write all in one `BEGIN IMMEDIATE ... COMMIT`. Any failure → `ROLLBACK` restores code to unconsumed. |
| Database-enforced uniqueness | `memberships_one_active` partial unique index on `(actor_id) WHERE left_at IS NULL`. Replaces application-level check. Race-proof. |
| Backfill authority | Backfilled role is informational until separately authorized provisioning completes. `control_token_hash = NULL` means playback-only via node token. |
