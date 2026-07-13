# Recovery and Telegram Link Contract

Date: 2026-07-12
Task: `TASK-260712-3v1k7q`
Status: Contract note for downstream implementation (revised per root reviews R1–R13)

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
  authoritative `slots` row for the node token hash.
  `installation_credentials.binding_token_hash` is an immutable duplicated
  snapshot of `slots.token_hash` at bind time, used solely for
  binding-generation identity and endpoint join predicates;
  `slots.token_hash` remains the authoritative store for node authentication.
- **Legacy coexistence.** Existing `members` and `slots` tables remain intact.
  Backfill is idempotent and never deletes/rewrites their role or token rows.
  Rollback to the previous coordinator with the feature flag off requires a
  fail-closed operational procedure (§17.11).
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
- **Attempt limits are concurrency-safe.** Every syntactically valid request
  atomically reserves an attempt counter AFTER authentication and bounded syntax
  validation, but BEFORE any hash work or expensive generation.
  Concurrent requests cannot all slip through a pre-check window. Counts are
  all attempts (not only failures) for simplicity and safety.
- **Every destructive credential mutation runs inside an explicit
  `BEGIN IMMEDIATE ... COMMIT` transaction.** Recovery consume (§5.2), recovery
  rotate (§7), link issuance (§10), and Telegram link consume (§11) each wrap
  their reads, lifecycle checks, hash verification, conditional writes, and
  audit inside one SQLite writer transaction. Rate-limit reservation is outside
  the transaction. On any error: `ROLLBACK`. `BEGIN IMMEDIATE` acquires the
  SQLite write lock immediately, serializing concurrent writers; whichever
  writer commits first wins, and subsequent writers see the committed state.
- **Recovery consume is bound to the exact recovery generation.** The conditional
  write predicates on `recovery_id` and `consumed_at IS NULL` — not merely
  `actor_id`. Within the `BEGIN IMMEDIATE` transaction, no concurrent writer can
  modify the row between read and update. The `RowsAffected()` check is a
  defensive safety net. A rotation that committed before the transaction started
  changes the stored `recovery_id`, so the submitted `recovery_id` does not
  match at lookup (step 5) and returns `credential_invalid`.
- **Single-row recovery rotation model.** Each `installation_credentials` row
  has exactly one current recovery generation. Rotation atomically overwrites
  `recovery_id`, `recovery_secret_hash`, and resets `consumed_at = NULL`. There
  is no multi-row generation history. The old `recovery_id` becomes permanently
  invalid. Idempotent replay of a consumed secret is valid only until the next
  rotation replaces it.
- **Telegram link consume is a rollback-safe transaction.** Actor resolution,
  conflict checks, code reservation, membership creation, legacy dual-write,
  and audit happen inside one SQLite `BEGIN IMMEDIATE` transaction. On failure
  paths within the transaction (constraint violation, conflict, error),
  `ROLLBACK` restores the code to its unconsumed state. A concurrent winner that
  has already committed makes the code permanently consumed; subsequent consumers
  see `consumed_at IS NOT NULL` at lookup and return `credential_invalid` without
  modifying the code — nothing to roll back.
- **Link issuance is serialized.** Invalidating prior codes and inserting a new
  code share one `BEGIN IMMEDIATE` transaction with the issuer
  capability/lifecycle check (§10). This gives issuance-vs-consume a frozen
  linearization.
- **No node-token control escalation.** A node token grants playback, heartbeat,
  and scoped media download only. It MUST NOT provision control tokens or
  recovery material. Legacy unprovisioned installations obtain control authority
  through a separately authorized flow (device invite from primary/companion,
  explicit Telegram-owner authorization, or another frozen proof).
- **Database-enforced uniqueness** for one-active-membership-per-actor: a partial
  unique index `ON memberships(actor_id) WHERE left_at IS NULL` replaces the
  prior application-level check. This is race-proof.
- **Dual-write coexistence uses conflict-safe UPSERT**, not `INSERT OR REPLACE`.
  The legacy `members` table has both a `(orbit_id, tg_user_id)` primary key
  and a global unique `members_user(tg_user_id)`. `INSERT OR REPLACE` could
  delete an existing foreign-orbit row. Instead, the Telegram link consume checks
  `members` by `tg_user_id` inside the transaction and rejects any foreign-orbit
  mismatch. Same-orbit writes use `INSERT ... ON CONFLICT(orbit_id, tg_user_id)
  DO UPDATE`; an unexpected `members_user` uniqueness conflict rolls back the
  entire transaction.
- **Reconciliation after rollback.** While `self_service_onboarding` is off,
  the legacy `members` AND `slots` tables are authoritative. On re-deployment
  with the flag on, idempotent reconciliation synchronizes additive tables with
  any changes the old coordinator made during rollback. The old coordinator can
  join, leave/delete, rename, change roles, pair, rebind, revoke, and dissolve;
  all are reconciled.
- **Conservative backfill.** Orphan/inconsistent legacy slots (no matching
  `paired_by` member) backfill as `satellite` — the lowest-capability role that
  cannot issue links or exercise control. Explicit authorized repair is required
  to upgrade.
- **`orbits.status` is an additive column** with database-enforced CHECK
  constraint. Live `orbits` has no `status` column; the migration adds
  `status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled'))`.
  All lifecycle rules that check orbit status reference this column.
- **`PRAGMA foreign_keys = ON`** is required on every database connection. FK
  constraints are declared on additive tables; existing tables (which have no
  `REFERENCES` clauses) are unaffected. Tests must prove FK violations on
  additive tables are rejected.
- **Authorization = token capability × role policy.** `satellite` is a
  membership role, not by itself proof that every control endpoint is impossible.
  Authorization comes from the combination of the token's capability class (node
  or control) and the role matrix. Unprovisioned installations
  (`control_token_hash = NULL`) authenticate via node token only and expose only
  node capability regardless of backfilled role.
- **In-transaction bearer re-authentication.** Every destructive control-token
  mutation (`/recovery/rotate`, `/telegram-links`) passes the presented token's
  hash into the store method and, inside `BEGIN IMMEDIATE`, verifies it still
  equals the current `control_token_hash` before any write. A token revoked by a
  concurrent recovery consume is rejected with `401 unauthorized`, preventing
  stale-bearer mutations.
- **Consume-time issuer authority validation.** Telegram link consume (§11)
  re-checks the code's issuer at consume time inside the transaction: active
  orbit, non-revoked issuer, active issuer membership in the exact target orbit,
  issuer role primary or companion. If the issuer was revoked, left, downgraded,
  or the orbit disabled after code issuance, consume fails with generic
  `credential_invalid` and the code is NOT consumed.
- **Same-orbit legacy member is already linked.** If the legacy `members` table
  has an active row for the consuming Telegram user in the target orbit, Telegram
  link consume returns `already_linked_same_orbit` and the code is NOT consumed.
  The code's `desired_role` never overwrites a migrated member's existing role.
  The feature becomes serving-ready only after startup reconciliation succeeds.
- **Executable DDL** (§17.9) provides the exact idempotent SQL for all additive
  tables, including `CHECK` constraints for column formats, orbit status,
  consumed-state consistency, `(kind, external_ref)` identity uniqueness for
  both actor kinds, partial unique indexes, recovery-field all-or-none
  consistency, FK declarations with explicit delete ordering, and timestamp
  units (Unix milliseconds).
- **Go `_txlock=immediate`** (§17.10) is added to the DSN so that every
  `s.db.Begin()` call issues `BEGIN IMMEDIATE`. With `SetMaxOpenConns(1)`, all
  queries within a transaction MUST use the `sql.Tx` handle (never `sql.DB`).
  modernc v1.53.0 supports this parameter. Tests assert `PRAGMA foreign_keys`,
  exercise two independent `Store` instances on the same DB, and run
  `PRAGMA foreign_key_check` after migration/reconciliation.
- **Full old-binary reconciliation** (§17.8) covers both `members` and `slots`.
  The old coordinator can transfer primary, leave/dissolve, rebind/re-pair slots,
  revoke slots, add members, and rename members. Reconciliation detects new
  slots, revoked slots, rebound slots (via `slot_paired_at` generation marker),
  role/ownership changes, membership additions, name changes, and orbit
  dissolution. A rebound slot conservatively revokes the old installation
  actor's credentials. While the feature flag is on, all legacy mutation methods
  dual-write to additive tables transactionally.
- **Slot rebind is generation-safe.** On rebind, the old actor retains its
  original generation-scoped `external_ref` and is revoked. The old
  `installation_credentials` row is deleted (it references a stale slot
  binding). A new actor and credential are created in the same transaction
  with a new `external_ref` containing the new binding fingerprint. The
  `actors_identity` unique index is satisfied because old and new
  `external_ref` values differ (different fingerprints). The binding
  fingerprint is a **domain-separated, versioned, full 64-character** lowercase
  hex SHA-256 digest:
  `SHA-256("barycenter/slot-binding/v1:" + token_hash)` (§17.6). On
  `(kind, external_ref)` conflict, the implementation verifies that the
  fingerprints match (true idempotent backfill) and fails closed on a mismatch.
- **Orbit-alignment invariant with live-slot binding validation.** An app
  actor's active membership must be in exactly
  `installation_credentials.slot_orbit_id`, the referenced slot must be
  unrevoked, and the slot's current `token_hash` must equal the credential's
  immutable `binding_token_hash`. Every normative endpoint algorithm (§5.2,
  §6, §7, §10) joins the credential's `slot_orbit_id` to
  `memberships.orbit_id`, joins `slots` with `s.revoked_at IS NULL`, AND
  verifies `s.token_hash = ic.binding_token_hash`. A revoked slot, stale
  binding (same-coordinate rebind not yet reconciled), or orbit mismatch
  fails the join and produces `401 unauthorized` (credential invalid) or
  `403 credential_invalid` (recovery consume). Authenticated endpoints use
  a **staged query**: stage 1 validates the credential/actor/binding (`401`
  on failure), stage 2 evaluates membership/role/orbit authority (`403` on
  failure). This prevents conflating stale credential binding with valid-token
  lifecycle errors. Migration/reconciliation fail closed on mismatch (§17.5).
- **Old-binary rollback is NOT unconditionally safe.** The old coordinator
  ignores `orbits.status` and `actors.revoked_at`, and its `PairSlot` can reuse
  revoked letters. The fail-closed rollback procedure (§17.11) projects disabled
  orbits into legacy state that the old binary enforces: revoke slots, set
  `max_pulsars = 0` / `max_members = 0` (prevents PairSlot/AddMember), and
  burn pending invites. Emergency rollback without projections requires keeping
  the affected tenants offline. Startup reconciliation plus
  `PRAGMA foreign_key_check` gate re-enabling new endpoints.
- **Atomic rate-limit reservation order is uniform.** For all endpoints:
  auth (if applicable) → bounded syntax validation → atomic reservation →
  generation (if applicable) → writer transaction. A `400 invalid_request`
  never touches the limiter. This eliminates the R8 ordering contradiction.
- **Pending recovery is scoped by coordinator origin + installation target.**
  The client's protected pending record is keyed by
  `(canonical_coordinator_origin, actor_id)` for the one-sent-candidate
  guarantee, plus `recovery_id` for generation specificity. `actor_id` is a
  non-secret integer included in the recovery export alongside `recovery_id`
  and `recovery_secret` (both create and rotate responses return `actor_id`).
  At most one unresolved `ever_sent` candidate may exist per
  `(canonical_coordinator_origin, actor_id)` — this prevents accumulating
  ambiguous pending tokens for the same installation, even across recovery
  generations. Starting a new recovery attempt while an `ever_sent` candidate
  is unresolved requires first promoting/superseding it via probe, or
  destructive-abandon with explicit user confirmation. Silent overwrite is
  forbidden. A lone `401` from probe is NEVER sufficient to auto-delete a
  pending candidate whose `ever_sent` is true. Canonical coordinator origin
  follows the HTML Living Standard origin serialization with IDNA2008/UTS46
  (§5.1.2); shared test vectors ensure cross-platform byte-identical keys.
- **Windows durable write uses `MoveFileExW`, not `ReplaceFile`.** The
  `REPLACEFILE_WRITE_THROUGH` flag on `ReplaceFileW` is documented as
  unsupported. The frozen algorithm uses `MoveFileExW` with
  `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH` (documented as
  supported), preceded by `FlushFileBuffers` on the temp file handle and
  followed by reopen + decrypt + field read-back verification of the
  destination. DPAPI MUST use **current-user scope** (omit
  `CRYPTPROTECT_LOCAL_MACHINE`; set `CRYPTPROTECT_UI_FORBIDDEN`). All
  `CreateFileW` parameters are frozen: `GENERIC_WRITE` + share mode `0` +
  `CREATE_NEW` for the temp file, `GENERIC_READ` + share mode `0` +
  `OPEN_EXISTING` for read-back. Write uses a complete-write loop with
  byte-count verification. Blob framing includes magic/version/length
  header with truncation and trailing-data rejection. `LocalFree` is called
  on every DPAPI `DATA_BLOB` output. The network request MUST NOT begin
  until read-back confirms the durable, correct `ever_sent = true` record
  (§5.1.2).

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
- `coordinator/internal/store/orbits.go:552-584` — live `PairSlot`: uses
  `INSERT OR REPLACE` with `time.Now().UnixMilli()` for `paired_at`. Two
  rebinds within the same millisecond produce identical `paired_at` values.
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
   authenticated Bot API transport (TLS connection to api.telegram.org) and the
   secrecy of the bot token, NOT from any cryptographic signature on the
   `Update` object itself (there is none for long-polling; webhook mode has a
   separate optional secret-token header). An in-process consumer of these
   updates can therefore trust `from.id` as a Telegram principal.
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
   serialized. `BEGIN IMMEDIATE` acquires the write lock at the start of the
   transaction, ensuring no concurrent writer can interleave reads and writes
   within the transaction boundary. The conditional `UPDATE` statement with its
   predicates provides the application-level invariant.
   Source: [SQLite File Locking](https://www.sqlite.org/lockingv3.html),
   [WAL mode](https://www.sqlite.org/wal.html),
   [BEGIN IMMEDIATE](https://www.sqlite.org/lang_transaction.html)

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

11. **SQLite `ALTER TABLE ADD COLUMN` with CHECK (verified locally).**
    SQLite accepts `ALTER TABLE t ADD COLUMN c TEXT NOT NULL DEFAULT 'x'
    CHECK(c IN ('x','y'))` and enforces the constraint on subsequent writes.
    Verified with SQLite 3.45.0 (modernc). This means `orbits.status` can be
    added with a CHECK constraint in a single idempotent migration statement.

12. **`paired_at` millisecond collision (verified against source).**
    Live `PairSlot` at line 583 uses `time.Now().UnixMilli()`. Two rapid
    rebinds of the same slot can produce identical `paired_at` values. This
    makes `paired_at` alone insufficient as a collision-proof generation
    marker for detecting every rebind. The contract uses a full 64-character
    SHA-256 binding fingerprint of `token_hash` (§17.6) to detect every rebind
    with 256-bit collision resistance. The prior 8-hex-char (32-bit) truncated
    fingerprint was insufficient: a 32-bit collision space permits detectable
    collisions in realistic attack scenarios.

13. **`MoveFileExW` with `MOVEFILE_WRITE_THROUGH` (verified against Microsoft
    documentation).** The `MoveFileExW` function supports
    `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH`. The documentation
    states: "Setting this value guarantees that a move performed as a copy and
    delete operation is flushed to disk before the function returns." In
    contrast, `ReplaceFileW` documents `REPLACEFILE_WRITE_THROUGH` as
    unsupported. This makes `MoveFileExW` the only documented write-through
    atomic-replace primitive on Windows for the pending-credential use case.
    Source: https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw
    Source: https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-replacefilew

14. **SQLite `orbits.status` rebuild sequence (verified against SQLite docs).**
    SQLite's recommended table schema change procedure is: create new table →
    copy data → drop old table → rename new table. The alternative sequence
    (rename old → create new → copy → drop old) is explicitly identified as
    incorrect because modern SQLite rewrites child FK `REFERENCES` declarations
    to follow the renamed table.
    Source: https://www.sqlite.org/lang_altertable.html#making_other_kinds_of_table_schema_changes

15. **Live `slots.paired_at` is nullable (verified against source).**
    `coordinator/internal/store/orbits.go:43` declares `paired_at INTEGER`
    with no `NOT NULL` constraint. `PairSlot` at line 583 always sets it to
    `time.Now().UnixMilli()`, but legacy or manually created rows may have NULL.
    The backfill sentinel `0` is safe because `time.Now().UnixMilli()` returns
    positive values (Unix epoch in milliseconds), and the binding fingerprint
    is the authoritative rebind detector regardless of `paired_at`.

16. **DPAPI current-user scope (verified against Microsoft documentation).**
    `CryptProtectData` with `dwFlags = 0` (or `CRYPTPROTECT_UI_FORBIDDEN`
    only) encrypts data using the current user's logon credentials. Omitting
    `CRYPTPROTECT_LOCAL_MACHINE` (0x4) ensures only the same user account on
    the same machine can decrypt. `CRYPTPROTECT_UI_FORBIDDEN` (0x1) prevents
    the API from displaying UI prompts, appropriate for non-interactive
    service or background contexts.
    Source: https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata

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
for node authentication. `installation_credentials.binding_token_hash` is an
immutable copy of `slots.token_hash` captured at bind time (§17.6); it serves
as a generation-identity snapshot for endpoint join predicates and collision
verification, not as an operational node-auth lookup column.

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
     "actor_id": 7,
     "recovery_id": "rec_a1b2c3d4e5f6789001a2b3c4d5e6f789",
     "recovery_secret": "ABCDEFGHJKMNPQRSTVWXYZ23456",
     "shown_once": true
   }
   ```

   `actor_id` is a non-secret integer identifying this installation. It is
   included in the recovery export so that a clean install can scope the
   pending-credential one-sent-candidate guarantee by
   `(canonical_coordinator_origin, actor_id)` (§5.1.2).

2. `recovery_id` is a stable non-secret handle for one installation
   credential row. The client MAY persist `recovery_id`, `actor_id`,
   `orbit_id`, issuance timestamps, and a local "user confirmed backup" flag.
3. The client MUST NOT silently persist `recovery_secret` beside node/control
   credentials, in app-private files, Keychain/DPAPI, logs, telemetry,
   analytics, clipboard history, pasteboard history, or crash reports.
4. Any explicit copy/save/export flow MUST package `actor_id`, `recovery_id`,
   and `recovery_secret` together; a clean install will otherwise lack the
   non-secret handles needed for lookup and pending-state scoping.
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
| `canonical_coordinator_origin` | string | Canonical coordinator origin: `{scheme}://{idna_lowercase_host}:{effective_port}` — no path, query, or fragment. Equivalent URLs (e.g., `https://coord.example.com` and `https://coord.example.com:443`) MUST resolve to the same canonical form. |
| `actor_id` | integer | Non-secret installation target from the recovery export. Scopes the one-sent-candidate guarantee to this specific installation. |
| `recovery_id` | string | Lookup handle for this recovery generation |
| `pending_control_token` | string (64 hex) | The replacement credential |
| `ever_sent` | boolean | Set to `true` immediately before the first network send; never reverted |

**Pending record scope:** the one-sent-candidate guarantee is scoped by
`(canonical_coordinator_origin, actor_id)`. At most one pending record with
`ever_sent = true` may exist per this tuple. `recovery_id` is stored in the
record for generation tracking. Two pending records with different
`(origin, actor_id)` tuples are fully independent.

**Protocol:**

1. **Generate:** Client generates a 256-bit replacement control token (32 bytes
   from platform CSPRNG, hex-encoded to 64 characters).
2. **Persist as pending:** Client writes
   `{canonical_coordinator_origin, actor_id, recovery_id, pending_control_token,
   ever_sent: false}` to Keychain (macOS) or DPAPI-protected file (Windows)
   BEFORE sending the request. The `recovery_secret` is NOT written; it remains
   user-supplied. `actor_id` comes from the recovery export.
3. **Mark sent and flush:** Client sets `ever_sent = true` in the protected
   record. On Windows, the durable write sequence is: `CreateFileW` temp file
   with `GENERIC_WRITE`, share mode `0`, `CREATE_NEW` → write
   current-user-DPAPI-encrypted blob → `FlushFileBuffers` → `CloseHandle` →
   `MoveFileExW(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)` → reopen
   destination with `GENERIC_READ` + `OPEN_EXISTING` → `CryptUnprotectData` →
   verify `ever_sent = true` and all expected fields (see §5.1.2 Persistence
   Primitives for the complete algorithm with exact `CreateFileW` parameters).
   On macOS, `SecItemUpdate` is individually atomic at the item level. **The
   network request in step 4 MUST NOT begin until durable storage confirms
   `ever_sent = true` via read-back verification.** A power loss between the
   non-durable write and send would lose the pending record while a committed
   server-side token exists.
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

#### 5.1.2 Double-Start Protection

At most one unresolved `ever_sent` candidate may exist per target installation
at any time. The one-sent-candidate guarantee is scoped by
`(canonical_coordinator_origin, actor_id)` — the canonical coordinator origin
identifies the server, and `actor_id` (a non-secret integer from the recovery
export) identifies the specific installation. This prevents a second recovery
attempt from silently overwriting a candidate that the server may have already
accepted. A coordinator origin is NOT an installation: one coordinator hosts
many app actors, so scoping by origin alone would either block unrelated actors
or fail to protect against cross-generation overwrites for the same actor.

**Canonical coordinator origin:** `{scheme}://{idna_lowercase_host}:{effective_port}`.
No path, query, or fragment. This follows the
[HTML Living Standard origin serialization](https://html.spec.whatwg.org/multipage/browsers.html#ascii-serialisation-of-an-origin)
with the specific IDNA and edge-case decisions frozen below.

Canonicalization algorithm:

1. **Scheme:** lowercase. Only `https` and `http` are accepted; any other scheme
   (e.g., `ftp`, `ws`, custom) is rejected as malformed.
2. **Userinfo:** rejected. A URL with `user:pass@host` is malformed.
3. **Host — IDNA:** apply IDNA2008 via UTS46 mapping
   (`UseSTD3ASCIIRules=true`, `Transitional_Processing=false`,
   `CheckBidi=true`, `CheckJoiners=true`). Convert to A-labels (Punycode).
   Lowercase the result. Both implementations (macOS/Windows) MUST use the
   same UTS46 profile and produce identical A-label output; otherwise pending
   records created on one platform cannot be resolved on the other.
4. **Trailing root dot:** strip. `coord.example.com.` → `coord.example.com`.
5. **IPv4:** normalize to decimal dotted quad (no octal, no hex, no
   single-integer forms). `127.0.0.1` is canonical.
6. **IPv6:** enclose in brackets per RFC 3986 §3.2.2. Normalize to
   RFC 5952 canonical form (lowercase hex, `::` compression in the leftmost
   longest run, no leading zeros). Zone identifiers (`%25eth0`) are rejected
   (zone IDs are link-local and not meaningful for coordinator origins).
   Example: `https://[::1]:8443`.
7. **Port:** omit default ports (443 for `https`, 80 for `http`). Include
   non-default ports as decimal integers. `https://host` and `https://host:443`
   MUST produce the same canonical form `https://host`.
8. **Path, query, fragment:** stripped entirely. Only `scheme://host[:port]` is
   retained.
9. **Malformed/opaque URLs:** rejected. The URL must parse as a valid
   scheme-host-port tuple. A URL that fails parsing, contains encoded
   characters in the host, or is not an absolute URL with an authority
   component is malformed and MUST NOT produce a canonical origin.

**Shared test vectors:** the following inputs MUST produce the same canonical
origin on both macOS and Windows. Implementations MUST pass all vectors:

| Input URL | Canonical origin |
|---|---|
| `https://coord.example.com` | `https://coord.example.com` |
| `https://coord.example.com:443` | `https://coord.example.com` |
| `https://coord.example.com:443/` | `https://coord.example.com` |
| `https://coord.example.com:8443` | `https://coord.example.com:8443` |
| `http://coord.example.com` | `http://coord.example.com` |
| `http://coord.example.com:80` | `http://coord.example.com` |
| `http://coord.example.com:8080` | `http://coord.example.com:8080` |
| `https://COORD.Example.COM` | `https://coord.example.com` |
| `https://coord.example.com.` | `https://coord.example.com` |
| `https://127.0.0.1` | `https://127.0.0.1` |
| `https://[::1]:8443` | `https://[::1]:8443` |
| `https://[0:0:0:0:0:0:0:1]:8443` | `https://[::1]:8443` |
| `https://münchen.example.com` | `https://xn--mnchen-3ya.example.com` |
| `https://coord.example.com/path?q=1#frag` | `https://coord.example.com` |
| `https://user:pass@coord.example.com` | *rejected (userinfo)* |
| `ftp://coord.example.com` | *rejected (unsupported scheme)* |
| `https://[::1%25eth0]:8443` | *rejected (zone ID)* |

Equivalent URLs MUST produce byte-identical canonical origins and therefore
byte-identical protected-store keys on both platforms.

**Rule:** before generating a new `pending_control_token` for a given
`(canonical_coordinator_origin, actor_id)`, the client MUST check for an
existing pending record with the same `(origin, actor_id)` and
`ever_sent = true`:

- If such a record exists: the client MUST first resolve it:
  1. **Probe** with the existing `pending_control_token` (§5.1.1).
  2. If probe returns `200` or `403 insufficient_capability`: **promote** the
     existing candidate (it is valid). Do not generate a new one.
  3. If probe returns `401`: the existing candidate was never committed or was
     superseded. The client MUST NOT auto-delete the pending record based on a
     lone `401`. Instead, retry recovery with the **same existing
     `(recovery_id, recovery_secret, pending_control_token)` tuple**. Only if
     the retry succeeds (200) → promote. If the retry returns `403` → offer
     the destructive-abandon Cancel (§5.1.1 Discard/Cancel rules). A `401`
     probe followed by `403` recovery retry in the same session, with user
     confirmation, is the only path to deletion.
  4. If probe returns `5xx`/network error: the candidate's server state is
     unknown. The client MUST NOT overwrite it. Retry probe later.
  5. If the user explicitly requests Cancel (destructive-abandon with warning):
     delete the existing pending record, then a new recovery attempt may proceed.
- If no `ever_sent` record exists for this `(origin, actor_id)` (or `ever_sent`
  is `false`): the client MAY safely replace the pending record with a new one.

**Different `(canonical_coordinator_origin, actor_id)` tuples** are fully
independent. A pending record for actor A does NOT block creating a pending
record for actor B, even on the same coordinator.

**Enforcement across recovery generations:** `recovery_id` uniquely identifies
a generation on a given coordinator. If the user rotates recovery material, the
new `recovery_id` is different. The pending record stores `recovery_id` for
generation tracking, but the one-sent-candidate guard is keyed by
`(origin, actor_id)`, not `(origin, recovery_id)`. A pending record for the
old `recovery_id` persists until resolved — it represents a potentially valid
credential regardless of the current recovery generation. Before attempting
recovery for a different `recovery_id` on the same `(origin, actor_id)`, the
existing candidate must be resolved first (probe → promote/retry/cancel). This
prevents accumulating multiple ambiguous pending tokens for the same
installation across recovery generations.

**Persistence primitives:**

- macOS: Keychain item operations (`SecItemAdd`, `SecItemUpdate`,
  `SecItemDelete`) are individually atomic at the item level. A crash between
  two item operations can leave partial state; the pending-state protocol
  tolerates this because `ever_sent` is the only flag that transitions
  monotonically and is written with a single `SecItemUpdate`.
- Windows: DPAPI protects bytes via `CryptProtectData`/`CryptUnprotectData`.
  DPAPI is not itself an atomic persistence layer; it encrypts bytes. The
  client MUST use an explicit durable write sequence for the `ever_sent`
  transition. `ReplaceFile`/`ReplaceFileW` MUST NOT be used: the
  `REPLACEFILE_WRITE_THROUGH` flag is documented as unsupported
  (https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-replacefilew),
  so there is no way to guarantee write-through semantics with `ReplaceFile`.
  Additionally, `ReplaceFile`'s backup-file recovery is not automatic — partial
  failure layouts require manual inspection and are not self-repairing.

  The frozen executable algorithm uses `MoveFileExW` with
  `MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH`, which IS documented
  as supported
  (https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw):

  **DPAPI scope:** the pending credential is per-interactive-user. DPAPI MUST
  use **current-user scope** by omitting the `CRYPTPROTECT_LOCAL_MACHINE` flag
  from `CryptProtectData`. Microsoft documents that data protected with
  `CRYPTPROTECT_LOCAL_MACHINE` can be decrypted by **any user on the computer**
  (https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata),
  which would expose the pending control token to other local accounts. The
  `CRYPTPROTECT_UI_FORBIDDEN` flag MUST be set to suppress any interactive
  prompts during encryption/decryption (the operation runs in a background
  recovery flow, not a user-facing dialog).

  **Blob framing:** the serialized pending state MUST be framed with a
  fixed-length header containing a magic number (4 bytes), format version
  (1 byte), and payload length (4 bytes, little-endian). Maximum payload
  is **16,384 bytes** (generous upper bound for the pending fields; DPAPI
  adds its own overhead). On read-back, reject any blob where: the DPAPI
  output is shorter than the header, the magic number mismatches, the version
  is unknown, the declared payload length exceeds the maximum, or the DPAPI
  output contains trailing data beyond the header + payload. This prevents
  interpreting truncated, partial, or corrupted blobs as valid state.

  **`LocalFree`:** the `DATA_BLOB` output buffer from `CryptProtectData` and
  `CryptUnprotectData` is allocated by the system and MUST be freed with
  `LocalFree` after use, even on error paths. Failure to free leaks memory
  on every recovery attempt.

  1. **Open temp file:** `CreateFileW` with:
     - `dwDesiredAccess = GENERIC_WRITE`
     - `dwShareMode = 0` (exclusive — no concurrent readers/writers)
     - `lpSecurityAttributes = NULL` (inherit process default)
     - `dwCreationDisposition = CREATE_NEW` (fail if temp file already exists;
       the random suffix makes collisions negligible, and failure means retry
       with a different suffix)
     - `dwFlagsAndAttributes = FILE_ATTRIBUTE_NORMAL | FILE_FLAG_WRITE_THROUGH`

     The temp file MUST be on the same NTFS volume as the destination
     (e.g., `{dest_path}.tmp.{random_hex_8}`). Same-volume is required for
     atomic rename semantics; `MoveFileExW` across volumes falls back to
     copy-and-delete, which is NOT atomic.

  2. **Write:** serialize the pending state (with `ever_sent = true`) into the
     framed blob format. Call `CryptProtectData` with `dwFlags = CRYPTPROTECT_UI_FORBIDDEN`
     (no `CRYPTPROTECT_LOCAL_MACHINE` — current-user scope). Write the DPAPI
     output to the temp file in a loop: call `WriteFile` and accumulate
     `lpNumberOfBytesWritten` until the complete DPAPI blob is written. If any
     `WriteFile` call returns success but writes **zero bytes**, treat it as an
     error — a zero-progress write would loop forever. If `WriteFile` fails or
     writes zero bytes: `CloseHandle` temp file handle, delete temp file,
     `LocalFree` the DPAPI buffer, and abort. After the write loop, verify that
     total bytes written equals the DPAPI blob length. Call `LocalFree` on the
     DPAPI `DATA_BLOB.pbData` output buffer.

  3. **Flush data:** call `FlushFileBuffers` on the temp file handle. This
     forces all buffered data pages to stable storage. Without this, the move
     at step 5 may produce a valid filename pointing to unflushed data pages.

  4. **Close:** `CloseHandle` on the temp file handle. The file is now complete
     and durable on disk.

  5. **Atomic durable replace:** `MoveFileExW(temp_path, dest_path,
     MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)`. The
     `MOVEFILE_WRITE_THROUGH` flag ensures the function does not return until
     the rename metadata is flushed to stable storage. After this call returns
     successfully, the destination path points to the complete, encrypted
     `ever_sent = true` blob with both data and metadata durable.

  6. **Read-back verification:** reopen the destination file with
     `CreateFileW`:
     - `dwDesiredAccess = GENERIC_READ`
     - `dwShareMode = 0` (exclusive — prevents concurrent modification during
       verification)
     - `dwCreationDisposition = OPEN_EXISTING` (fail if file is missing — this
       should not happen after step 5)
     - `dwFlagsAndAttributes = FILE_ATTRIBUTE_NORMAL`

     **Ciphertext size cap:** call `GetFileSizeEx` immediately after opening.
     Reject the file if the size is zero (empty/corrupt) or exceeds
     **1,048,576 bytes** (1 MB — a conservative upper bound; the actual DPAPI
     ciphertext for the pending state fields is a few KB). A corrupt or
     locally replaced multi-gigabyte file MUST NOT cause unbounded memory
     allocation. On rejection: `CloseHandle`, abort, report error.

     Read the file in a complete-read loop (accumulate bytes until EOF or
     expected size). Reject if fewer bytes are read than `GetFileSizeEx`
     reported (premature EOF) or if the read exceeds the capped size. Call
     `CryptUnprotectData` with
     `dwFlags = CRYPTPROTECT_UI_FORBIDDEN`. Call `LocalFree` on the DPAPI
     output after copying the plaintext. Validate blob framing: magic number,
     format version, payload length within bounds, no trailing data.
     Deserialize and verify that the record contains exactly the expected
     `ever_sent = true`, `pending_control_token`, `recovery_id`, `actor_id`,
     and `canonical_coordinator_origin`. If ANY check fails — file too short,
     magic mismatch, version unknown, trailing data, decryption failure, field
     mismatch — **do NOT send**. Abort and report an error. `CloseHandle` on
     the read handle.

  7. **Cleanup:** delete any stale `.tmp.*` files in the same directory from
     prior crashed attempts (best-effort; failure to clean up is not blocking).

  **Resource ownership and cleanup table.** Every allocated resource MUST be
  released on every exit path — success and every failure branch. The table
  below defines the exact cleanup obligations:

  | Resource | Acquired at | Released by | Failure behavior |
  |---|---|---|---|
  | Temp file handle | Step 1 (`CreateFileW`) | Step 4 (`CloseHandle`) | On failure at steps 2–3: `CloseHandle` temp handle, then delete temp file, then abort. |
  | DPAPI encrypt output (`DATA_BLOB.pbData`) | Step 2 (`CryptProtectData`) | `LocalFree` after write loop completes (step 2) | On write failure: `LocalFree` before abort. On encrypt failure (`CryptProtectData` returns FALSE): if `DATA_BLOB.pbData` is non-NULL, `LocalFree` it; it may be partially allocated. |
  | Temp file on disk | Step 1 (created) | Step 5 (`MoveFileExW` consumes it) or explicit delete on failure | On failure at steps 2–4: delete temp file. On step 5 failure: delete temp if still present. Step 7 (cleanup): delete any orphaned `.tmp.*` files. |
  | Read handle | Step 6 (`CreateFileW` reopen) | Step 6 (`CloseHandle` after read-back) | On any read-back failure (size check, read, decrypt, framing, field validation): `CloseHandle` read handle before abort. A `CloseHandle` failure on the read handle is **fatal for this attempt**: record the OS error, do NOT send, and escalate/restart the flow. The handle state after a failed `CloseHandle` is unknown, so the attempt cannot claim proven cleanup and cannot proceed. |
  | DPAPI decrypt output (`DATA_BLOB.pbData`) | Step 6 (`CryptUnprotectData`) | `LocalFree` after copying plaintext fields | On decrypt failure: `LocalFree` if `DATA_BLOB.pbData` is non-NULL. On framing/field failure after successful decrypt: `LocalFree` the decrypt output before abort. |
  | Destination file | Exists before step 5 (old state) or created by step 5 | Persists (it IS the credential store) | On read-back failure (step 6): destination file is retained as-is (may be corrupt). The implementation retries from step 1 on the next recovery attempt. A corrupt destination is NOT automatically deleted — the user may retry or Cancel. |

  **Detailed failure handling at each step:**

  - Step 1 (`CreateFileW` fails): no resource was acquired. No state change.
    Safe to retry with a different random suffix.
  - Step 2a (`CryptProtectData` fails): `LocalFree` the DPAPI output if
    non-NULL. `CloseHandle` temp handle. Delete temp file. Abort.
  - Step 2b (`WriteFile` returns success with zero bytes written): treat as
    error — a zero-progress write would loop forever. `CloseHandle` temp
    handle. Delete temp file. `LocalFree` the DPAPI buffer if not yet freed.
    Abort.
  - Step 2c (`WriteFile` fails): same cleanup as 2b.
  - Step 2d (total bytes written != DPAPI blob length after loop): same
    cleanup as 2b.
  - Step 3 (`FlushFileBuffers` fails): `CloseHandle` temp handle. Delete
    temp file. Abort. (`LocalFree` on DPAPI buffer already done at step 2.)
  - Step 4 (`CloseHandle` fails on temp handle): the temp file may be
    orphaned but MUST NOT be moved. Abort before `MoveFileExW`. Clean up
    orphaned temp on next attempt (step 7).
  - Step 5 (`MoveFileExW` fails): the old destination file is unchanged (still
    contains `ever_sent = false` or the prior state). The temp file may or may
    not still exist — delete it if present. Abort.
  - Step 6a (`CreateFileW` reopen fails — file missing): should not happen
    after step 5; abort. No handle to close.
  - Step 6b (`GetFileSizeEx` fails): `CloseHandle` read handle. Abort.
  - Step 6c (file size is zero or exceeds 1 MB cap): `CloseHandle` read
    handle. Abort. Report "corrupt or oversized credential file."
  - Step 6d (`ReadFile` error or zero-progress read): `CloseHandle` read
    handle. Abort.
  - Step 6e (fewer bytes read than `GetFileSizeEx` reported — premature EOF):
    `CloseHandle` read handle. Abort.
  - Step 6f (`CryptUnprotectData` fails): `LocalFree` decrypt output if
    non-NULL. `CloseHandle` read handle. Abort.
  - Step 6g (framing invalid — magic mismatch, unknown version, payload
    length exceeds max, trailing data): `LocalFree` decrypt output (plaintext
    copy already taken or not needed). `CloseHandle` read handle. Abort.
  - Step 6h (field validation fails — `ever_sent` not true, token mismatch,
    wrong `recovery_id`, wrong `actor_id`, wrong origin): same cleanup as 6g.
    Abort. **Do NOT send.**
  - Step 6i (`CloseHandle` on read handle fails): **fatal for this attempt —
    do NOT send.** Even when the preceding read-back validated the data, a
    failed `CloseHandle` leaves the handle in an unknown state, so the attempt
    cannot assert proven cleanup. Record the OS error and escalate/restart the
    flow rather than sending on unproven cleanup. (A `CloseHandle` failure
    after a failed read-back is moot — send was already blocked.) This is the
    single frozen policy: **every** step-6 failure, including read-handle
    `CloseHandle` failure, blocks the network send.

  In summary: at no exit path does any DPAPI `DATA_BLOB` output remain leaked;
  `LocalFree` is called exactly once per `DATA_BLOB` allocation. `CloseHandle`
  is attempted exactly once per handle. If any `CloseHandle` reports failure,
  the attempt does not claim the handle was cleanly released and does not
  send — it records the OS error and escalates/restarts. The contract does not
  assert that a handle is always successfully closed; it asserts that no send
  proceeds unless every resource operation, close included, succeeded.

  **The network request MUST NOT begin until step 6 confirms the durable,
  correct `ever_sent = true` record.** This is the send barrier: steps 1–6
  must all succeed before step 4 of the protocol (Send).

  Power-loss behavior: at no point between steps 1–5 can the destination file
  contain partial data. Before step 5 succeeds, the old destination is intact
  (`ever_sent = false`). After step 5 succeeds with `MOVEFILE_WRITE_THROUGH`,
  the destination contains the complete temp file data (`ever_sent = true`)
  with durable metadata. `FlushFileBuffers` at step 3 ensures the data pages
  are also durable. Together, these guarantee that after any power loss, the
  destination contains either the old or new complete blob.

**Test requirements:** crash barriers at every write/flush/move/read-back/send
edge: before pending write, after temp-file write but before `FlushFileBuffers`,
after flush but before close, after close but before `MoveFileExW`, after move
but before read-back, after read-back but before send, after send but before
response, after active promotion, after pending deletion. Tests MUST assert:
`CryptProtectData` is called WITHOUT `CRYPTPROTECT_LOCAL_MACHINE` (current-user
scope), WITH `CRYPTPROTECT_UI_FORBIDDEN`; `CreateFileW` for temp uses
`GENERIC_WRITE`, share mode `0`, `CREATE_NEW`; `CreateFileW` for read-back uses
`GENERIC_READ`, share mode `0`, `OPEN_EXISTING`; `LocalFree` is called on every
DPAPI `DATA_BLOB` output; write loop verifies total bytes equals blob length;
read-back rejects truncated blobs, unknown magic, unknown version, trailing
data, and field mismatch.

**DPAPI fault-injection tests (R12 requirement):**
- **Oversized ciphertext:** create a destination file >1 MB, reopen at step 6,
  verify `GetFileSizeEx` check rejects it and `CloseHandle` is called. Assert
  "network not called."
- **Zero-size ciphertext:** create a zero-byte destination file, verify step 6
  rejects it.
- **`GetFileSizeEx` error:** inject `GetFileSizeEx` failure, verify
  `CloseHandle` read handle, abort, "network not called."
- **Short read / zero-progress read:** inject `ReadFile` returning fewer bytes
  than expected size (premature EOF), verify abort. Inject `ReadFile` returning
  success with zero bytes, verify treated as error (not infinite loop).
- **`ReadFile` error:** inject `ReadFile` failure, verify `CloseHandle` read
  handle, abort.
- **`CryptUnprotectData` failure with output allocation:** inject decrypt
  failure where `DATA_BLOB.pbData` is non-NULL (partial allocation). Verify
  `LocalFree` called on decrypt output AND `CloseHandle` called on read handle.
- **`CryptUnprotectData` failure without output allocation:** inject decrypt
  failure where `DATA_BLOB.pbData` is NULL. Verify no `LocalFree` on NULL
  pointer, `CloseHandle` on read handle.
- **Framing/field error after successful decrypt:** inject a blob with valid
  DPAPI envelope but wrong magic/version/field values. Verify `LocalFree`
  called on decrypt output, `CloseHandle` on read handle, abort.
- **Temp file `CloseHandle` failure (step 4):** inject `CloseHandle` failure
  on temp handle. Verify abort before `MoveFileExW`, orphaned temp file
  cleaned up on next attempt.
- **Read handle `CloseHandle` failure (step 6i):** inject `CloseHandle`
  failure on read handle after an otherwise-successful read-back. Verify the
  single frozen result: the OS error is recorded, the attempt is fatal, the
  flow escalates/restarts, and **the network is NOT called**. Assert exact
  resource counts (`LocalFree` count for the decrypt output, `CloseHandle`
  attempted once on the read handle) and "network not called" — consistent
  with the rule that every step-6 failure blocks the send.
- **`WriteFile` zero-byte write:** inject `WriteFile` returning success with
  `lpNumberOfBytesWritten = 0`. Verify treated as error, cleanup performed.
- All fault tests MUST assert exact `CloseHandle`, `LocalFree`, and delete
  counts, plus "network not called" for every failure path.

Double-start
test: start recovery, mark `ever_sent`, simulate no response, attempt to start
again for the same `(canonical_coordinator_origin, actor_id)` — the client must
probe first and refuse to overwrite. Test: request A sent and paused before
commit → probe returns `401` → user starts again → request A commits → the
client must still retain and promote A (the pending record was NOT deleted after
probe `401`). Power-loss test: verify that after power loss between temp-file
write and `MoveFileExW`, the old `ever_sent=false` blob survives (network
request never began, no server-side commit). Read-back verification test:
corrupt the destination file after move, verify step 6 detects the mismatch
and blocks the network send. Same-actor cross-generation test:
pending record for `recovery_id` R1 with `ever_sent=true`, attempt recovery
with `recovery_id` R2 for the same `(origin, actor_id)` — must resolve R1
first. Different-actor test: pending records for different `actor_id` values
on the same coordinator are independent.

#### 5.2 Server-Side Transaction

Processing order:

1. Parse and validate input format. Reject syntactically invalid input with
   `400 invalid_request`. No attempt counter is touched.
2. Source-IP attempt counter: atomically reserve (increment) and check. Reject
   if over limit with `429`. The increment and check MUST be a single atomic
   operation (e.g., `sync/atomic` or mutex-protected counter).
3. Per-`recovery_id` attempt counter: atomically reserve (increment) and check.
   Reject if over limit with `429`.
4. **`BEGIN IMMEDIATE`** — acquire SQLite write lock. All subsequent reads,
   checks, writes, and audit happen inside this transaction. On any error at
   any step below: **`ROLLBACK`** and return the error response.
5. Look up the credential row by `recovery_id`:
   ```sql
   SELECT actor_id, slot_orbit_id, recovery_secret_hash, consumed_at,
          control_token_hash
   FROM installation_credentials
   WHERE recovery_id = ?
   ```
   Both `actor_id` and `slot_orbit_id` are retained for the success response
   (`{orbit_id, actor_id, role}`) — `slot_orbit_id` provides `orbit_id`.
6. **If not found:** compute `hashToken(submitted_secret)` and constant-time
   compare against `dummy_hash`. **`ROLLBACK`**. Return
   `403 credential_invalid`.
7. **If found — check lifecycle state with orbit-alignment join** (inside the
   same transaction):
   a. Look up the actor's `revoked_at`, the membership's `left_at` and `role`,
      and the orbit's `status`, joining through the credential's `slot_orbit_id`
      to enforce the orbit-alignment invariant (§17.5) and the binding
      predicate:
      ```sql
      SELECT a.revoked_at, m.left_at, m.role, o.status,
             ic.slot_orbit_id AS orbit_id
      FROM installation_credentials ic
      JOIN actors a ON a.id = ic.actor_id
      JOIN memberships m ON m.actor_id = ic.actor_id
        AND m.orbit_id = ic.slot_orbit_id
      JOIN orbits o ON o.id = ic.slot_orbit_id
      JOIN slots s ON s.orbit_id = ic.slot_orbit_id
        AND s.slot = ic.slot_name
        AND s.revoked_at IS NULL
        AND s.token_hash = ic.binding_token_hash
      WHERE ic.actor_id = ?
      ```
      `orbit_id` is selected from the transaction-consistent row for the
      success response. Although `slot_orbit_id` was read at step 5, selecting
      it again here proves all response fields come from a consistent
      transaction snapshot; downstream code uses this single query's result.
      The `s.token_hash = ic.binding_token_hash` predicate ensures the
      credential's binding is current — a same-coordinate rebind before
      reconciliation fails this join. If the join produces no rows (e.g.,
      slot revoked, binding stale, orbit-alignment mismatch, no active
      membership in the credential's orbit): treat as lifecycle failure —
      compute dummy comparison, **`ROLLBACK`**, return
      `403 credential_invalid`.
   b. If the actor is revoked (`revoked_at IS NOT NULL`), the membership is
      left (`left_at IS NOT NULL`), or the orbit is disabled
      (`status = 'disabled'`): compute `hashToken(submitted_secret)` and
      constant-time compare against `dummy_hash` (do NOT compare against the
      real hash — this prevents confirming the secret is correct for an unusable
      context). **`ROLLBACK`**. Return `403 credential_invalid`.
8. **If not yet consumed (`consumed_at IS NULL`):**
   a. Compute `hashToken(submitted_secret)` and constant-time compare against
      stored `recovery_secret_hash`.
   b. If no match: **`ROLLBACK`**. Return `403 credential_invalid`.
   c. If match — **generation-bound conditional write (single-winner):**
      ```sql
      UPDATE installation_credentials
      SET consumed_at = ?,
          control_token_hash = ?
      WHERE actor_id = ?
        AND recovery_id = ?
        AND consumed_at IS NULL
      ```
      Within `BEGIN IMMEDIATE`, no concurrent writer can have modified this row
      between the read at step 5 and this update. `RowsAffected()` must be 1.
      If defensively 0 (should not happen within a correctly held write lock):
      **`ROLLBACK`**, return `403 credential_invalid`.
   d. Write audit event (no plaintext).
   e. **`COMMIT`**. Return `200 OK` with `{orbit_id, actor_id, role}` using
      `slot_orbit_id` from step 5, `actor_id` from step 5, and `role` from
      step 7a — all read inside this transaction.
9. **If already consumed (`consumed_at IS NOT NULL`) — idempotency check**
   (same transaction, no reload needed because state was read at step 5 inside
   the write lock):
   a. The submitted `recovery_id` matches the stored `recovery_id` (lookup was
      by this key). This confirms no rotation occurred between the original
      consume and this retry — if a rotation had committed before this
      transaction, the lookup would have found no row (the old `recovery_id` no
      longer exists) and step 6 would have returned `credential_invalid`.
   b. Re-check lifecycle: actor revoked, membership left, orbit disabled. A
      context that was subsequently disabled/revoked must reject even idempotent
      replay. If revoked/left/disabled: compute dummy comparison.
      **`ROLLBACK`**. Return `403 credential_invalid`.
   c. Compute `hashToken(submitted_secret)` and constant-time compare against
      stored `recovery_secret_hash`.
   d. If no match: **`ROLLBACK`**. Return `403 credential_invalid`.
   e. Compute `hashToken(replacement_control_token)` and constant-time compare
      against stored `control_token_hash`.
   f. If no match: **`ROLLBACK`**. Return `403 credential_invalid`. This
      prevents a different client from replaying with a different token.
   g. If both match: **idempotent success**. **`COMMIT`** (no mutation). Return
      `200 OK` with `{orbit_id, actor_id, role}` using `slot_orbit_id` from
      step 5, `actor_id` from step 5, and `role` from step 9b lifecycle
      re-check — all transaction-consistent.

Both the secret and the replacement token are verified on idempotent replay
(step 9c–9g) against the transaction-consistent row data. An attacker with only
the pending token (e.g., Keychain access without the recovery secret) cannot
complete recovery.

**Single-winner guarantee:** `BEGIN IMMEDIATE` acquires the SQLite write lock
at the start of the transaction. Only one writer can hold the lock at a time;
concurrent `BEGIN IMMEDIATE` callers block until the lock is released. Within
the transaction, all reads see a stable snapshot and no concurrent writer can
modify the row between the read (step 5) and the conditional update (step 8c).
The generation-bound `WHERE recovery_id = ? AND consumed_at IS NULL` predicate
is a logical invariant; `RowsAffected()` is a defensive check. Exactly one
concurrent consumer commits for a given `recovery_id`; subsequent consumers
see the committed consumed state (step 9) or, after rotation, see no row
(step 6).

**Consume-vs-rotate linearization:** both operations use `BEGIN IMMEDIATE` on
the same database. SQLite serializes them. If rotation commits first, it
replaces the row's `recovery_id`. When consume starts its transaction, step 5
finds no row for the old `recovery_id` → step 6 returns `credential_invalid`.
If consume commits first, it sets `consumed_at`. When rotation starts, it
overwrites the generation regardless of `consumed_at` state, issuing a fresh
secret. No stale consume can overwrite a new generation's `control_token_hash`.

**Consume-vs-revoke/leave/disable linearization:** if a revocation, leave, or
disable commits before the consume transaction starts, step 7 sees the
revoked/left/disabled state inside the transaction and rejects with
`credential_invalid`. If consume commits first, the credential is already
consumed when the revocation takes effect — the control token was already
replaced. Both orderings are safe.

**Recovery consume response completeness test requirement:** generate the
first-consume and consumed-replay test fixtures directly from the exact
`SELECT` statements at steps 5 and 7a. Assert that all three response fields
(`orbit_id`, `actor_id`, `role`) are populated with non-default values (e.g.,
`orbit_id` > 0, `actor_id` > 0, `role` in `('primary','companion','satellite')`)
and that `orbit_id` matches the credential's `slot_orbit_id`. Do not paper over
a missing column value with an out-of-transaction lookup or a hardcoded default.

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
  The check applies to both first consume (step 7) and idempotent replay
  (step 9b).
- Successful recovery does NOT auto-rotate a new recovery secret. The fresh
  control token may call `POST /v1/recovery/rotate` if the user explicitly
  chooses to create new recovery material.
- Concurrent attempts: exactly one transaction may consume a given
  `(recovery_id, recovery_secret)` pair successfully via the `BEGIN IMMEDIATE`
  transaction. Any concurrent consumer blocks until the winner commits, then
  sees the consumed state. The idempotent-retry case (step 9g) succeeds only
  with the exact same tuple.
- **Idempotency lifetime after rotation:** a consumed recovery secret's
  idempotent replay (step 9g) is valid only until the next
  `/recovery/rotate` replaces the row's `recovery_id` and
  `recovery_secret_hash`. After rotation, the old `recovery_id` is gone and
  retries with it fail at step 6 with `credential_invalid`. This is safe
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
- For control-token authentication, the actor context query uses a **staged
  query** that separates credential validation (`401`) from lifecycle
  authorization (`403`). This prevents conflating invalid/stale credential
  binding with valid-token lifecycle errors:

  **Stage 1 — Credential validation (determines 401 vs further checks):**
  ```sql
  SELECT ic.actor_id, ic.slot_orbit_id, ic.slot_name, a.revoked_at
  FROM installation_credentials ic
  JOIN actors a ON a.id = ic.actor_id
  JOIN slots s ON s.orbit_id = ic.slot_orbit_id
    AND s.slot = ic.slot_name
    AND s.revoked_at IS NULL
    AND s.token_hash = ic.binding_token_hash
  WHERE ic.control_token_hash = ?
  ```
  The `s.token_hash = ic.binding_token_hash` predicate ensures the
  credential's binding is current — a same-coordinate rebind that has not
  yet been reconciled will fail this join because `slots.token_hash` changed
  while `ic.binding_token_hash` still holds the old value. If the join
  produces no rows (token not found, slot revoked, binding stale, or orbit
  mismatch): `401 unauthorized`. If `a.revoked_at IS NOT NULL`:
  `401 unauthorized` (revocation invalidates the token itself).

  **Stage 2 — Lifecycle authorization (determines 403 vs 200):**
  ```sql
  SELECT m.orbit_id, m.role, m.left_at, o.status
  FROM memberships m
  JOIN orbits o ON o.id = m.orbit_id
  WHERE m.actor_id = ? AND m.orbit_id = ? AND m.left_at IS NULL
  ```
  (Using `actor_id` and `slot_orbit_id` from stage 1.) If membership not
  found, `m.left_at IS NOT NULL`, or `o.status = 'disabled'`:
  `403 insufficient_capability`. Otherwise: `200 OK` with
  `{orbit_id, actor_id, role}`.
- For node-token authentication (fallthrough to `LookupToken`), the query
  starts from the unrevoked slot and resolves the current credential/actor
  using an **INNER JOIN** with the binding predicate:
  ```sql
  SELECT s.orbit_id, s.slot, ic.actor_id, m.role, a.revoked_at,
         m.left_at, o.status
  FROM slots s
  JOIN installation_credentials ic
    ON ic.slot_orbit_id = s.orbit_id AND ic.slot_name = s.slot
    AND ic.binding_token_hash = s.token_hash
  JOIN actors a ON a.id = ic.actor_id
  LEFT JOIN memberships m ON m.actor_id = ic.actor_id
    AND m.orbit_id = s.orbit_id AND m.left_at IS NULL
  LEFT JOIN orbits o ON o.id = s.orbit_id
  WHERE s.token_hash = ? AND s.revoked_at IS NULL
  ```
  If the slot is not found or revoked: `401 unauthorized`. If the INNER JOIN
  on `installation_credentials` fails (no matching credential row):
  `401 unauthorized` — this should not happen after the serving gate because
  reconciliation ensures every unrevoked slot has a credential row. If an
  actor/membership exists, the same lifecycle checks as the control-token
  stage 2 apply (`403 insufficient_capability` for left/disabled).

  **Serving gate invariant:** after startup reconciliation, every unrevoked
  slot in an **active** orbit MUST have a corresponding
  `installation_credentials` row with a current binding. The startup
  sequence asserts this before enabling endpoints:
  ```sql
  SELECT s.orbit_id, s.slot FROM slots s
  JOIN orbits o ON o.id = s.orbit_id AND o.status = 'active'
  LEFT JOIN installation_credentials ic
    ON ic.slot_orbit_id = s.orbit_id AND ic.slot_name = s.slot
    AND ic.binding_token_hash = s.token_hash
  WHERE s.revoked_at IS NULL AND ic.actor_id IS NULL
  ```
  If any rows are returned: reconciliation is incomplete. Log a fatal error
  and do NOT enable the feature flag. This eliminates the "unbackfilled
  legacy slot" branch — every valid node token resolves to a complete
  `(actor_id, role)` context after reconciliation.
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
  "actor_id": 7,
  "recovery_id": "rec_a1b2c3d4e5f6789001a2b3c4d5e6f789",
  "recovery_secret": "ABCDEFGHJKMNPQRSTVWXYZ23456",
  "shown_once": true
}
```

`actor_id` is included so the client can construct the
`(canonical_coordinator_origin, actor_id)` pending-recovery scope (§5.1.2)
without needing a separate identity lookup. The recovery export at create
time (§4) also includes `actor_id` for the same reason. Both create and
rotate MUST return `actor_id`; round-trip tests MUST verify the field is
present and matches the authenticated actor.

Rules:

- Node tokens MUST fail with `403 insufficient_capability`.
- Rotation MUST be an explicit user action. Create and recover flows MUST NOT
  silently call it.
- Revoked actors: `401 unauthorized` (token revoked).
- Left membership: `403 insufficient_capability`.
- Disabled orbit: `403 insufficient_capability`.
- Rate limit: 10 authenticated, syntactically valid attempts per actor per
  rolling 60 minutes (§9).

**Server-side transaction:**

1. Auth middleware resolves bearer token to `actor_id` and computes
   `presented_token_hash = hashToken(bearer_token)` (outside transaction).
   Node tokens fail with `403 insufficient_capability`.
2. Validate request body (bounded parse, max 64 bytes, must be `{}`).
   Invalid → `400 invalid_request`, no counter touch.
3. Rate limit: atomically reserve an attempt for this `actor_id` (outside
   transaction). If over limit: `429 too_many_attempts`.
4. Generate new recovery material: `recovery_id` (16 random bytes, hex,
   `rec_` prefix), `recovery_secret` (`generateSecret(27)`), compute
   `recovery_secret_hash = hashToken(secret)`. Generation uses only
   `crypto/rand` and is done outside the transaction.
5. **`BEGIN IMMEDIATE`** — acquire SQLite write lock.
6. **Staged credential validation and lifecycle check** inside the transaction.

   **Stage 1 — Credential validation (determines 401):**
   ```sql
   SELECT ic.control_token_hash, ic.slot_orbit_id, a.revoked_at
   FROM installation_credentials ic
   JOIN actors a ON a.id = ic.actor_id
   JOIN slots s ON s.orbit_id = ic.slot_orbit_id
     AND s.slot = ic.slot_name
     AND s.revoked_at IS NULL
     AND s.token_hash = ic.binding_token_hash
   WHERE ic.actor_id = ?
   ```
   If the join produces no rows (slot revoked, binding stale, credential
   missing): **`ROLLBACK`**, return `401 unauthorized`.
   If `a.revoked_at IS NOT NULL`: **`ROLLBACK`**, return `401 unauthorized`.

7. **Bearer re-authentication:** constant-time compare `presented_token_hash`
   with `control_token_hash` from stage 1. If mismatch (e.g., a concurrent
   recovery consume replaced the control token between middleware auth and
   `BEGIN IMMEDIATE`): **`ROLLBACK`**, return `401 unauthorized`. This prevents
   a stale, revoked token from performing mutations.

8. **Stage 2 — Lifecycle authorization (determines 403):**
   ```sql
   SELECT m.role, m.left_at, o.status
   FROM memberships m
   JOIN orbits o ON o.id = m.orbit_id
   WHERE m.actor_id = ? AND m.orbit_id = ? AND m.left_at IS NULL
   ```
   (Using `actor_id` and `slot_orbit_id` from stage 1 — both are explicitly
   selected.)
   If membership not found, left, orbit disabled, or role is `satellite`:
   **`ROLLBACK`**, return `403 insufficient_capability`.

9. **Single-row overwrite:**
    ```sql
    UPDATE installation_credentials
    SET recovery_id = ?,
        recovery_secret_hash = ?,
        consumed_at = NULL
    WHERE actor_id = ?
    ```
    - `recovery_id` is replaced with the new 128-bit handle. The previous
      `recovery_id` becomes permanently invalid for lookup and consume.
    - `recovery_secret_hash` is replaced with the SHA-256 hash of the new secret.
    - `consumed_at` is reset to `NULL` (the new secret is unconsumed).
    - `control_token_hash` is NOT modified. The current control token remains
      valid.
    - Node token, membership, role, and actor identity are NOT modified.
10. Write audit event: log old `recovery_id` (non-secret handle) being replaced
    and new `recovery_id` being issued. No plaintext secret in audit.
11. **`COMMIT`**. Return `200 OK` with the new recovery material.

On any error during steps 5–10: **`ROLLBACK`**, return appropriate error.

**Stale-token barrier test:** authenticate with token A → pause → recovery
consume commits token B (revoking A by overwriting `control_token_hash`) →
resume rotation with A → step 7 detects mismatch → `401 unauthorized`, no
rotation. Also test: authenticate A → pause → role change to satellite → resume
→ step 8 detects satellite → `403 insufficient_capability`.

**Rotation-vs-consume linearization:**

Both operations use `BEGIN IMMEDIATE` on the same database, serializing writers.
If rotation commits first, consume's lookup (§5.2 step 5) finds no row for the
old `recovery_id` → `credential_invalid`. If consume commits first, rotation
proceeds normally — it overwrites `recovery_id` and `recovery_secret_hash`,
resets `consumed_at`, and the consumed secret becomes non-replayable (its
`recovery_id` no longer matches any row).

**Rotation-vs-revoke linearization:**

If revocation commits before the rotation transaction starts, stage 1 step 6
sees `revoked_at IS NOT NULL` and rejects. If rotation commits first, the new
recovery material is issued and a subsequent revocation invalidates the actor.

**Collision retry:** if the newly generated `recovery_id` collides with a
`UNIQUE` constraint (astronomically unlikely), generate-and-retry.

**Idempotency:** rotation is NOT idempotent. Each call generates new material
with a new `recovery_id`. If the response is lost, the user may call rotate
again because the control token remains valid. The server MUST NOT keep
replayable plaintext copies.

**Idempotent replay lifetime of consumed secrets:** after rotation, any
idempotent replay of the old consumed secret (§5.2 step 9) fails because the
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

Every syntactically valid request atomically reserves (increments) an attempt
counter AFTER authentication (for authenticated endpoints) and bounded syntax
validation, but BEFORE any database lookup, hash verification, or expensive
generation work. This prevents a race where many concurrent requests all pass a
pre-check window before any failure is recorded. The counter counts ALL
syntactically valid attempts (not only failures) — this is simpler,
concurrency-safe, and ensures each attempt consumes a slot regardless of
outcome. This rule applies uniformly to all endpoints: unauthenticated consume
endpoints AND authenticated mutation endpoints (rotation, link issuance).

**Limiter processing order (unauthenticated endpoints — recovery/invite consume):**

1. Input format validation → `400` (no counter touch).
2. Source-IP attempt counter: atomically increment → if counter exceeds limit,
   return `429`. The increment and check MUST be a single atomic operation.
3. Per-key attempt counter (e.g., `recovery_id`): atomically increment → if
   counter exceeds limit, return `429`.
4. `BEGIN IMMEDIATE` → database lookup + SHA-256 comparison (or dummy
   comparison for unknown IDs) → `COMMIT` or `ROLLBACK`.

**Limiter processing order (authenticated endpoints — rotation, link issuance):**

1. Auth middleware resolves bearer token to `actor_id` and computes
   `presented_token_hash`. Invalid/missing token → `401` (no counter touch).
2. Input format validation (bounded body parse, field checks) → `400`
   (no counter touch). For rotation: validate body is `{}` (max 64 bytes).
   For link issuance: validate `desired_role` is `companion` or `satellite`
   (max 256 bytes body); `primary` or unknown → `400 invalid_request`.
3. Per-`actor_id` attempt counter: atomically increment → if counter exceeds
   limit, return `429`. The increment and check MUST be a single atomic
   operation. This happens AFTER validation but BEFORE material generation or
   `BEGIN IMMEDIATE`.
4. Generate material (rotation: recovery ID/secret; issuance: link code).
5. `BEGIN IMMEDIATE` → re-auth, lifecycle checks, mutation → `COMMIT` or
   `ROLLBACK`.

**Rate limit table:**

| Endpoint | Key | Window | Limit | Counts |
|---|---|---|---|---|
| `POST /v1/recovery/consume` | source IP | 15 min rolling | 30 | All syntactically valid attempts |
| `POST /v1/recovery/consume` | `recovery_id` | 15 min rolling | 10 | All syntactically valid attempts |
| `POST /v1/recovery/rotate` | `actor_id` (from token) | 60 min rolling | 10 | All authenticated, syntactically valid attempts |
| `POST /v1/telegram-links` | `actor_id` (from token) | 60 min rolling | 10 | All authenticated, syntactically valid attempts |
| Telegram link consume | `telegram_user_id` (from Update) | 15 min rolling | 10 | All syntactically valid attempts |
| `POST /v1/device-invites/consume` | source IP | 15 min rolling | 20 | All syntactically valid attempts |

**What counts as an attempt (for rotation and issuance):**

- A successfully authenticated request with syntactically valid input: counts.
- `401 unauthorized` from middleware (missing/invalid bearer): does NOT count
  (not authenticated → no `actor_id` to key on).
- `400 invalid_request` (bad input format after auth): does NOT count (not
  syntactically valid — rejected at step 2 before reservation at step 3).
- `429 too_many_attempts`: the reservation itself WAS the increment; the
  response is the rejection. The attempt is already counted.
- `401` from in-transaction bearer re-auth (stale token): DOES count — the
  request was authenticated at middleware time and syntactically valid.
- `403 insufficient_capability` (lifecycle/role): DOES count.
- Collision retry (astronomically rare, internal): does NOT count separately;
  it is part of the same attempt.
- Response-loss retry from the same user: the server receives it as a new
  request; it counts normally against the same `actor_id`. The higher limit
  (10 vs former 3/5) accommodates legitimate retries.

**Bounded limiter keys:**

- A syntactically valid fake `recovery_id` (matching `^rec_[0-9a-f]{32}$` but
  corresponding to no database row) CAN create an LRU limiter key. Format
  validation alone does not bound keys to database rows.
- The explicit **10,000-entry LRU cap** per endpoint bounds total limiter state.
  An entry expires when its window closes. Entries are evicted LRU when the cap
  is reached. This limits an attacker's ability to exhaust limiter memory with
  arbitrary syntactically valid IDs.
- The source-IP limiter applies to every syntactically valid attempt before any
  per-key counter or hash work — including unknown IDs.
- Format-invalid inputs (`400 invalid_request`) never touch the limiter.
- Authenticated endpoint limiters key on `actor_id` from middleware auth, which
  is a database-backed integer — not an arbitrary client-supplied value. No LRU
  cap is needed for `actor_id`-keyed limiters.

**Source-IP extraction:**

The source IP is the **direct TCP peer address** (`RemoteAddr` after stripping
port). Spoofable forwarding headers (`X-Forwarded-For`, `X-Real-IP`,
`Forwarded`) are NOT trusted unless the request arrived through an **explicitly
configured trusted reverse proxy** (an allowed-proxy list in coordinator
configuration). If a trusted proxy is configured, use the rightmost
non-trusted IP from the forwarding chain. In Phase 1 (direct TLS or loopback),
no trusted proxy is configured and `RemoteAddr` is authoritative.

**Barrier tests at last available slot:**

For each limiter, test that the N-th attempt (where N = limit) succeeds and the
(N+1)-th returns `429 too_many_attempts`. Test with concurrent requests at the
boundary to verify atomic reservation prevents overrun.

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

**Server-side transaction:**

1. Auth middleware resolves bearer token to `actor_id` and computes
   `presented_token_hash = hashToken(bearer_token)` (outside transaction).
   Node tokens fail with `403 insufficient_capability`.
2. Validate `desired_role` input (bounded body parse, max 256 bytes).
   `primary` → `400 invalid_request`. Unknown value → `400 invalid_request`.
   Missing/empty → default `companion`. Invalid JSON or body too large →
   `400 invalid_request`. No counter touch for any `400`.
3. Rate limit: atomically reserve an attempt for this `actor_id` (outside
   transaction). If over limit: `429 too_many_attempts`.
4. Generate link code and compute hash: `code = generateSecret(27)`,
   `code_hash = hashToken(code)`. Done outside the transaction.
5. **`BEGIN IMMEDIATE`** — acquire SQLite write lock.
6. **Staged credential validation and lifecycle check** inside the transaction,
   identical to the rotate pattern (§7 steps 6–8):

   **Stage 1 — Credential validation (determines 401):**
   ```sql
   SELECT ic.control_token_hash, ic.slot_orbit_id, a.revoked_at
   FROM installation_credentials ic
   JOIN actors a ON a.id = ic.actor_id
   JOIN slots s ON s.orbit_id = ic.slot_orbit_id
     AND s.slot = ic.slot_name
     AND s.revoked_at IS NULL
     AND s.token_hash = ic.binding_token_hash
   WHERE ic.actor_id = ?
   ```
   If the join produces no rows (slot revoked, binding stale, credential
   missing): **`ROLLBACK`**, return `401 unauthorized`.
   If `a.revoked_at IS NOT NULL`: **`ROLLBACK`**, return `401 unauthorized`.

7. **Bearer re-authentication:** constant-time compare `presented_token_hash`
   with `control_token_hash` from stage 1. If mismatch: **`ROLLBACK`**,
   return `401 unauthorized`.

8. **Stage 2 — Lifecycle authorization (determines 403):**
   ```sql
   SELECT m.role, m.left_at, o.status
   FROM memberships m
   JOIN orbits o ON o.id = m.orbit_id
   WHERE m.actor_id = ? AND m.orbit_id = ? AND m.left_at IS NULL
   ```
   (Using `actor_id` and `slot_orbit_id` from stage 1.)
   If membership not found, left, orbit disabled, or role is `satellite`:
   **`ROLLBACK`**, return `403 insufficient_capability`.
9. Invalidate prior unconsumed codes for the same issuer:
    ```sql
    UPDATE telegram_link_codes
    SET invalidated_at = ?
    WHERE issuer_actor_id = ?
      AND consumed_at IS NULL
      AND invalidated_at IS NULL
    ```
10. Insert new code:
    ```sql
    INSERT INTO telegram_link_codes
      (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, created_at)
    VALUES (?, ?, ?, ?, ?, ?)
    ```
11. Write audit event (no plaintext code).
12. **`COMMIT`**. Return `201 Created` with response body.

On any error during steps 5–11: **`ROLLBACK`**, return appropriate error.

**Stale-token barrier test:** authenticate with token A → pause → recovery
consume commits token B → resume issuance with A → step 7 detects mismatch →
`401 unauthorized`, no code issued, prior codes not invalidated.

**Issuance-vs-consume linearization:**

Invalidating prior codes (step 10) and inserting the new code (step 11) share
one `BEGIN IMMEDIATE` transaction with the issuer lifecycle/bearer check
(steps 6–9). This ensures a Telegram link consume that uses `BEGIN IMMEDIATE`
(§11) is serialized against issuance: either the consume sees the new code and
the old code is already invalidated, or the consume committed before issuance
started and the old code is already consumed. No interleaving is possible.

Rules:

- `link_code` uses `generateSecret(27)` (132.49-bit, rejection-sampled).
- TTL is exactly 15 minutes from issue time.
- The `issuer_actor_id` on the `TelegramLinkCode` row is the issuing app actor.
  It is not an ownership transfer to Telegram.
- Collision retry: if `code_hash` collides with a `UNIQUE` constraint
  (astronomically unlikely), generate-and-retry outside the transaction.

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
On failure paths within the transaction, `ROLLBACK` restores the code to its
unconsumed state. A concurrent winner that has already committed makes the code
permanently consumed; subsequent consumers see the committed state.

Processing order (all inside the transaction unless stated):

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
   and `expires_at > ?` (TTL). If not found: **`ROLLBACK`**, compute
   `hashToken(submitted_code)` vs `dummy_hash` (timing equalization), return
   `credential_invalid`.
5. **Validate issuer authority at consume time** (inside the same transaction).
   The code was legitimately issued, but the issuer's context may have changed:
   a. Read `actors.revoked_at` for `issuer_actor_id`.
   b. Read `memberships.role`, `memberships.left_at` for the issuer's membership
      in `orbit_id` (the code's target orbit).
   c. Read `orbits.status` for `orbit_id`.
   d. If any of the following: issuer actor revoked, issuer membership not found,
      issuer membership left, issuer role is `satellite`, orbit disabled —
      **`ROLLBACK`**, compute `hashToken(submitted_code)` vs `dummy_hash`
      (timing equalization), return `credential_invalid`. Code NOT consumed.
   e. Verify stored `desired_role` is `companion` or `satellite` (defense in
      depth against tampered data; should be guaranteed by issuance validation).
6. **Hash-verify the code:** compute `hashToken(normalized_code)` and
   constant-time compare against `code_hash`. If mismatch: **`ROLLBACK`**,
   return `credential_invalid`.
7. **Resolve or create the Telegram actor:**
   - Query `actors` for `kind = 'telegram_user'` and
     `external_ref = '{telegram_user_id}'`.
   - If found and `revoked_at IS NOT NULL`: **`ROLLBACK`**, return
     `credential_invalid`. Revocation is stronger than re-linking.
   - If not found: `INSERT INTO actors (kind, external_ref, display_name, created_at)
     VALUES ('telegram_user', ?, ?, ?)`. Use the returned `actor_id`.
   - If found and not revoked: reuse the existing `actor_id`. Update
     `display_name` if changed.
8. **Check existing membership (both new and legacy tables):**

   a. Check the additive `memberships` table:
      ```sql
      SELECT orbit_id, role FROM memberships
      WHERE actor_id = ? AND left_at IS NULL
      ```
      - If active membership in the **same orbit** as the code's `orbit_id`:
        **`ROLLBACK`**, return `already_linked_same_orbit`. Code NOT consumed.
      - If active membership in a **different orbit** (Phase 1: one orbit per
        Telegram user): **`ROLLBACK`**, return
        `telegram_member_of_other_orbit`. Code NOT consumed.

   b. Check the legacy `members` table:
      ```sql
      SELECT orbit_id FROM members WHERE tg_user_id = ?
      ```
      - If found in a **different orbit** from the code's `orbit_id`:
        **`ROLLBACK`**, return `telegram_member_of_other_orbit`. Code NOT
        consumed.
      - If found in the **same orbit** as the code's `orbit_id`:
        **`ROLLBACK`**, return `already_linked_same_orbit`. Code NOT consumed.
        This Telegram user is already a member of the target orbit via legacy
        membership. The code's `desired_role` MUST NOT overwrite the migrated
        member's existing role. If additive state is missing or divergent from
        legacy, reconciliation (§17.8) — not an unauthenticated link code — is
        the repair path.

9. **Reserve the code (conditional write):**
   ```sql
   UPDATE telegram_link_codes
   SET consumed_at = ?, consuming_actor_id = ?
   WHERE code_hash = ?
     AND consumed_at IS NULL
     AND invalidated_at IS NULL
     AND expires_at > ?
   ```
   Check `RowsAffected()`: within `BEGIN IMMEDIATE`, the row state is stable
   since our read at step 4. `RowsAffected()` must be 1. If defensively 0
   (should not happen within a correctly held write lock): **`ROLLBACK`**,
   return `credential_invalid`.
10. **Create or reactivate membership:**
    - If the actor had a previous membership in this orbit with
      `left_at IS NOT NULL`: reactivate with `left_at = NULL`,
      `joined_at = now`, at the code's `desired_role`. This is a re-join after
      a previous leave, not escalation of a migrated member.
    - Otherwise: `INSERT INTO memberships (orbit_id, actor_id, role, joined_at)
      VALUES (?, ?, ?, ?)`.
    - If the INSERT fails (e.g., partial unique index violation — defense in
      depth; normally caught at step 8): **`ROLLBACK`**, return
      `credential_invalid`. The code is restored to unconsumed.
11. **Dual-write legacy `members` table** (§17 coexistence) using conflict-safe
    UPSERT:
    ```sql
    INSERT INTO members (orbit_id, tg_user_id, role, joined_at, display_name)
    VALUES (?, ?, ?, ?, ?)
    ON CONFLICT(orbit_id, tg_user_id) DO UPDATE
      SET role = excluded.role,
          joined_at = excluded.joined_at,
          display_name = excluded.display_name
    ```
    This UPSERT handles the re-join case (reactivation after leave) without
    deleting foreign-orbit rows. The `ON CONFLICT` clause targets the primary
    key `(orbit_id, tg_user_id)`. If an unexpected uniqueness conflict occurs
    on the `members_user(tg_user_id)` index (different orbit — should have been
    caught at step 8b), the INSERT fails, causing **`ROLLBACK`** and returning
    `credential_invalid`. The code is restored to unconsumed.
    Note: step 8b ensures that same-orbit legacy members are rejected before
    reaching this point, so the UPSERT's `DO UPDATE` clause only fires on
    re-join (where a prior leave left the legacy row intact but the additive
    membership was marked `left_at`).
12. **Audit** the consume event (no plaintext code in payload).
13. **`COMMIT`** — all-or-nothing. Return success with
    `{orbit_id, actor_id, role}`.

**Same-code/two-user race:**

With `BEGIN IMMEDIATE`, two consumers of the same code are serialized. The
first consumer acquires the write lock, processes steps 3–13, and commits.
The code is permanently consumed (`consumed_at IS NOT NULL`). The second
consumer then acquires the write lock, reads the code at step 4, and finds
`consumed_at IS NOT NULL` — the lookup returns no rows. The second consumer
receives `credential_invalid`. The code **remains consumed** (the winner's
commit is final); nothing is restored for the loser because the loser made
no writes. This is the correct behavior: the code was legitimately consumed
by the winner.

**Two-code/same-user race:**

Two different link codes consumed concurrently by the same Telegram user.
With `BEGIN IMMEDIATE`, the first consumer acquires the write lock, processes,
creates/reuses the actor, creates the membership, commits. The second consumer
then acquires the write lock, reads its own code (still unconsumed — it is a
different code), resolves the same Telegram actor at step 7, and at step 8a
finds the active membership created by the first consumer. The second consumer
returns `already_linked_same_orbit` (or `telegram_member_of_other_orbit` if
the codes target different orbits). **`ROLLBACK`**. The second code is NOT
consumed — the reservation at step 9 was never reached (or if it was, the
`ROLLBACK` restores it). The partial unique index
`memberships_one_active ON memberships(actor_id) WHERE left_at IS NULL` serves
as defense in depth but is not normally the rejection mechanism under
serialized writers.

**Issue→revoke/leave/satellite/disable→consume race:**

A code is issued while the issuer has active primary/companion authority.
Before the code is consumed, the issuer's authority changes (revoked, left,
downgraded to satellite, or orbit disabled). The consumer acquires the write
lock, reads the code at step 4 (code exists and is unconsumed), then at step 5
reads the issuer's current lifecycle state inside the same transaction. Step 5d
detects the changed state and returns `credential_invalid`. Code NOT consumed.
Test both orderings: (a) lifecycle change commits before consume starts; (b)
lifecycle change commits after consume reads the code but before it checks the
issuer — impossible under `BEGIN IMMEDIATE` because only one writer holds the
lock, so (a) is the only reachable ordering. The issuer lifecycle check at
consume time ensures codes become invalid the moment the issuer loses authority.

**Same-orbit legacy member rejection:**

A legacy member who is already in the target orbit attempts to consume a link
code (or someone consumes a code on behalf of a Telegram user who already has a
legacy `members` row in the target orbit). Step 8b detects the same-orbit legacy
row and returns `already_linked_same_orbit`. Code NOT consumed. The member's
existing role is preserved. Test cases: legacy-only same-orbit primary,
legacy-only same-orbit companion, additive-only (no legacy row), foreign-orbit,
and role divergence between legacy and additive tables — proving no role or row
is moved and no code is consumed on conflict.

**Bot message hygiene:**

- The bot MUST NOT echo the link code back to the user in any message.
- On successful consume, the bot SHOULD best-effort delete the user's message
  containing the code (using Telegram `deleteMessage` API where permitted by
  chat permissions). Failure to delete is not an error.
- Error messages from the bot use the same generic language as the JSON error
  envelope — no information about why the code failed.

### 12. Concurrent Consume and Replay

- **Recovery:** exactly one `BEGIN IMMEDIATE` transaction may consume a given
  `(recovery_id, recovery_secret)` pair. Concurrent consumers are serialized by
  the write lock: the winner commits, subsequent consumers see the consumed
  state and enter the idempotency path (§5.2 step 9). Same-tuple replay returns
  idempotent success (step 9g) — but only until the next rotation replaces the
  `recovery_id`. A rotation that committed before the transaction started causes
  the lookup to find no row → `credential_invalid`. Different-tuple replay or
  different replacement token returns `403 credential_invalid` (step 9f).
- **Telegram link:** exactly one `BEGIN IMMEDIATE` transaction may consume a
  given `link_code`. Concurrent consumers are serialized: the winner commits
  and the code is permanently consumed; the loser sees `consumed_at IS NOT NULL`
  at lookup (§11 step 4) and returns `credential_invalid` without modifying the
  code. On failure paths within a single transaction (constraint violation,
  conflict at step 7, foreign-orbit mismatch), `ROLLBACK` restores the code to
  unconsumed. Post-success replays return `credential_invalid`.
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
| Node token | **Preserved** in `slots.token_hash`. If the installation still has it locally, it keeps working. If lost, the installation is in control-only state until rebind. `installation_credentials.binding_token_hash` is an immutable snapshot of `slots.token_hash` at bind time, used for generation identity and endpoint joins (§17.6); `slots.token_hash` remains the authoritative node-auth store. |
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
  `slot_orbit_id INTEGER NOT NULL`, `slot_name TEXT NOT NULL` (together
  reference `slots(orbit_id, slot)` for the authoritative node token hash;
  rows are DELETED on rebind/revoke, so these columns are never NULL),
  `slot_paired_at INTEGER NOT NULL` (generation marker: copied from
  `slots.paired_at` at backfill/creation time; used by reconciliation §17.8 to
  detect rebinds),
  `binding_token_hash TEXT NOT NULL` (64-char hex, immutable copy of
  `slots.token_hash` at bind time; used in endpoint SQL joins to validate
  current binding and as an independent collision verifier in §17.6;
  `slots.token_hash` remains the authoritative node-auth store),
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
  slot via `(slot_orbit_id, slot_name)` and stores an immutable copy in
  `binding_token_hash` for generation identity (§17.6).

**Revocation mechanism (Phase 1):** replacing the current `control_token_hash`
on the `installation_credentials` row is the sole control-token revocation
mechanism. The previous hash is overwritten; no history record is kept. Admin
revocation of the actor itself (`actors.revoked_at`) makes ALL authentication
fail regardless of token validity. No control token version/history table
exists in Phase 1.

**Authorization = token capability × role policy:**

`satellite` is a membership role, not by itself proof that every control
endpoint is impossible. Authorization is determined by the combination of:

- **Token capability class:** node tokens grant playback, heartbeat, and
  media-download capability only. Control tokens grant all node capabilities
  plus admin, upload, and provisioning.
- **Role policy:** the authorization matrix (§10, and the capability×role table
  below) specifies which roles can perform which operations.

An unprovisioned installation (`control_token_hash = NULL`,
`recovery_id = NULL`) authenticates only via node token → node capability.
No control operation can succeed regardless of backfilled role.

**Capability × role matrix:**

| Operation | Node token | Control + primary | Control + companion | Control + satellite |
|---|---|---|---|---|
| Playback, heartbeat, media download | Yes | Yes | Yes | Yes |
| `GET /v1/actor/context` | Yes (read-only) | Yes | Yes | Yes |
| `POST /v1/recovery/rotate` | `403` | Yes | Yes | `403` |
| `POST /v1/telegram-links` | `403` | Yes | Yes | `403` |
| `POST /v1/recovery/consume` | N/A (unauthenticated) | — | — | — |
| Upload / admin operations | `403` | Yes | Yes | `403` |

### 17. Additive Schema / Legacy Coexistence

**Core rule:** existing `members` and `slots` tables remain intact and
unmodified by the migration. Backfill is idempotent and never deletes or
rewrites their role or token rows. Rollback to the previous coordinator with
the feature flag off requires a fail-closed operational procedure (§17.11).

#### 17.1 `orbits.status` Additive Column

Live `orbits` has no `status` column. Every lifecycle rule in this contract
queries `orbits.status`. The additive migration adds:

```sql
ALTER TABLE orbits ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
  CHECK(status IN ('active', 'disabled'))
```

Allowed values: `'active'`, `'disabled'`. Existing orbits are `'active'` by
default after migration. The old coordinator ignores unknown columns; schema
readability is compatible.

**Migration error handling — three cases:**

1. **Column does not exist:** execute the `ALTER TABLE ADD COLUMN` statement
   above. It succeeds and the CHECK constraint is active for all subsequent
   writes. This is the normal fresh-migration path.

2. **Column already exists with the CHECK constraint:** `ALTER TABLE ADD COLUMN`
   returns `SQLITE_ERROR` (error code 1, message contains "duplicate column
   name"). This specific error is safe to ignore (idempotent).

3. **Column already exists WITHOUT the CHECK constraint** (left by a partial
   rollout or manual intervention): ignoring the "duplicate column" error leaves
   the column writable with arbitrary values (e.g., `status = 'bogus'`). This
   is NOT safe. The migration MUST detect this case using a **behavior probe**
   (not a substring search of `sqlite_master.sql`) and repair it:

   a. **Behavior probe:** determine whether the existing column is constrained
      by testing actual database behavior. Within a `SAVEPOINT`:
      - If the table is empty: attempt `INSERT INTO orbits(title, created_at,
        status) VALUES('__probe__', 0, 'bogus')`. If the INSERT succeeds, the
        constraint is absent. `ROLLBACK TO SAVEPOINT`. (The `SAVEPOINT`
        prevents the probe row from persisting.)
      - If the table has rows: attempt `UPDATE orbits SET status = 'bogus'
        WHERE id = (SELECT MIN(id) FROM orbits)`. If the UPDATE succeeds, the
        constraint is absent. `ROLLBACK TO SAVEPOINT`.
      - If the INSERT/UPDATE fails with a CHECK constraint error: the
        constraint is present and effective. `ROLLBACK TO SAVEPOINT`.
      `RELEASE SAVEPOINT` after the probe.

      A substring search of `sqlite_master.sql` is NOT sufficient: it is
      whitespace-sensitive, can be fooled by comments containing the search
      text, and cannot distinguish equivalent from non-equivalent constraint
      expressions. A rolled-back behavior probe tests the actual enforcement.

   b. If the behavior probe shows the CHECK is absent: validate all existing
      values with
      `SELECT COUNT(*) FROM orbits WHERE status NOT IN ('active', 'disabled')`.
      If any invalid values exist: **abort startup** with a fatal error naming
      the invalid rows. Do not silently repair data of unknown origin.

   c. If all values are valid: rebuild the table with the constrained schema.
      The safe idempotent rebuild sequence follows the SQLite-documented
      correct order (create-new → copy → drop-old → rename-new), NOT the
      unsafe rename-old-first order which rewrites child FK declarations:

      **Step 1 — Capture and classify dependent objects.** Before any
      destructive operation, query `sqlite_master` for all schema objects
      that may reference `orbits`. The query MUST select `tbl_name` for
      ownership classification — without it, there is no column to classify
      by:
      ```sql
      SELECT type, name, tbl_name, sql FROM sqlite_master
      WHERE type IN ('index', 'trigger', 'view')
        AND sql IS NOT NULL;
      ```
      Classify each result by **schema ownership** (`tbl_name`), NOT by a
      substring/LIKE match on `sql`. A `sql LIKE '%orbits%'` filter is NOT
      a dependency parser: it is whitespace-sensitive, matches comments and
      string literals containing the word "orbits," and cannot distinguish
      owned objects from unrelated ones. Instead, use the deterministic
      `tbl_name` column:

      - **Indexes and triggers owned by `orbits`** (`tbl_name = 'orbits'`):
        automatically removed by `DROP TABLE orbits`. They need only
        recreation at step 3. Filter out auto-indexes (name starting with
        `sqlite_`).
      - **All user-defined views** (`type = 'view'`): are NOT auto-dropped by
        `DROP TABLE` (SQLite documents that `DROP TABLE` removes only
        associated indexes and triggers, not views:
        https://www.sqlite.org/lang_droptable.html). Their exact `sql` DDL is
        captured, and **every** user-defined view is dropped before the table
        rebuild and recreated after the rename.
      - **All external triggers** (`type = 'trigger'` AND
        `tbl_name != 'orbits'`): these are NOT auto-dropped. Their exact `sql`
        DDL is captured, and **every** external trigger is dropped before the
        table rebuild and recreated after the rename.

      **No text, substring, `LIKE`, or token analysis of `sql` bodies is
      performed.** R13/R14 rejected any prose "does the body reference orbits"
      heuristic because it has no defined lexer, quoting rules, schema-
      qualification rules, or false-negative policy (a quoted/qualified/CTE
      reference could be missed). Instead this migration takes the conservative
      strategy R14 named: classify strictly by object `type` and `tbl_name`,
      then capture and recreate **all** user-defined views and **all** external
      triggers unconditionally — never a subset selected by scanning `sql`.
      Recreating an object that would otherwise have been preserved is
      impossible under this rule because every user view and every external
      trigger is dropped inside the same transaction before recreation, so no
      "already exists" collision can occur and no true dependency can be
      silently omitted. Auto-indexes (`name` beginning `sqlite_`) are excluded;
      SQLite recreates them automatically. This is deterministic and
      executable: the object set to drop/recreate is fully determined by the
      step-1 query result with no ambiguity.

      **Step 2 — Rebuild with exact known schema.** The migration uses the
      exact known live `orbits` column list (from the live Go schema at
      `coordinator/internal/store/orbits.go:19-27`), NOT a column list derived
      from `PRAGMA table_info`. `table_info` does not preserve CHECK
      constraints, foreign keys, collations, generated columns, or table
      options; deriving `CREATE TABLE` from it can produce an incorrect schema:
      ```sql
      -- Capture the caller's current foreign_keys setting first (SELECT via
      -- `PRAGMA foreign_keys`) so it can be restored on EVERY exit path; the
      -- global contract (§ "PRAGMA foreign_keys = ON") requires enforcement on
      -- for every connection. See the defer/finally rule below the block.
      PRAGMA foreign_keys = OFF;
      BEGIN IMMEDIATE;
      -- Drop EVERY user-defined view (unconditionally; they are NOT
      -- auto-dropped). Execute DROP VIEW for each captured view from step 1.
      DROP VIEW IF EXISTS <captured_view_name>;
      -- Drop EVERY external trigger (unconditionally; NOT auto-dropped).
      DROP TRIGGER IF EXISTS <captured_external_trigger_name>;
      CREATE TABLE orbits_new (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL,
        takeover_policy TEXT NOT NULL DEFAULT 'user',
        voice_default TEXT NOT NULL DEFAULT 'personal',
        max_pulsars INTEGER NOT NULL DEFAULT 5,
        max_members INTEGER NOT NULL DEFAULT 10,
        created_at INTEGER NOT NULL,
        status TEXT NOT NULL DEFAULT 'active'
          CHECK(status IN ('active', 'disabled'))
      );
      INSERT INTO orbits_new SELECT id, title, takeover_policy,
        voice_default, max_pulsars, max_members, created_at, status
        FROM orbits;
      DROP TABLE orbits;
      ALTER TABLE orbits_new RENAME TO orbits;
      -- Step 3: recreate ALL captured objects INSIDE the transaction
      -- (indexes owned by orbits, EVERY user view, EVERY external trigger).
      -- Execute each captured CREATE statement from step 1 here, in the
      -- capture order (indexes and owned triggers first, then views/triggers
      -- whose bodies may depend on them).
      CREATE INDEX ... ;  -- each captured index
      CREATE VIEW ... ;   -- each captured view
      CREATE TRIGGER ... ; -- each captured trigger
      -- Step 4: validate INSIDE the transaction, before COMMIT.
      -- Run PRAGMA foreign_key_check; if any violations, ROLLBACK and abort.
      -- Run the behavior probe (INSERT status='bogus' in a SAVEPOINT,
      -- verify CHECK error, ROLLBACK TO SAVEPOINT). If probe succeeds
      -- (no constraint), ROLLBACK the transaction and abort — rebuild failed.
      COMMIT;
      PRAGMA foreign_keys = ON;
      ```
      Reference: https://www.sqlite.org/lang_altertable.html#making_other_kinds_of_table_schema_changes
      — SQLite explicitly identifies the rename-old-first sequence as incorrect
      because modern SQLite rewrites child FK `REFERENCES` declarations to
      follow the renamed table. Renaming `orbits` to `orbits_backup` first
      would rewrite `memberships REFERENCES "orbits_backup"(id)`, causing
      `DROP TABLE orbits_backup` to fail with a foreign-key constraint error
      when FKs are enabled.

      `PRAGMA foreign_keys` cannot be toggled inside a transaction, so it is
      set to `OFF` before `BEGIN`. Restoring it only after `COMMIT` is a bug:
      if the transaction takes any `ROLLBACK`/abort/error/panic path, the
      connection is left with FK enforcement OFF, violating the global contract
      that every connection has `foreign_keys = ON`. A rolled-back control
      reproduction (`.research/root-checks/recovery-r14-foreign-keys.sql`:
      `PRAGMA foreign_keys=ON; PRAGMA foreign_keys=OFF; BEGIN IMMEDIATE;
      CREATE TABLE t(...); ROLLBACK; PRAGMA foreign_keys;`) returns `0`,
      confirming the ROLLBACK path leaves it OFF.

      **Freeze one defer/finally restoration.** Before setting
      `PRAGMA foreign_keys = OFF`, read and capture the caller's current
      setting (`SELECT` via `PRAGMA foreign_keys`; the contract requires it to
      be `1`). Register a deferred restoration (Go `defer`, or the equivalent
      `try/finally`) that re-executes `PRAGMA foreign_keys = <captured>` — and,
      because the contract mandates enforcement, asserts the restored value is
      `1` — and this restoration MUST run on **every** exit from the migration
      function:
      - after `COMMIT` (success),
      - after `ROLLBACK` from a failed `foreign_key_check`,
      - after `ROLLBACK` from a failed behavior probe,
      - after any SQL error at any statement (the defer rolls back an open
        transaction first, then restores),
      - on a panic/exception boundary (the defer runs during unwind), and
      - on the interrupted-migration intermediate-state abort (step d).
      Because `PRAGMA foreign_keys` cannot execute inside an open transaction,
      the defer first ensures no transaction is open (issue `ROLLBACK`,
      ignoring "no transaction" errors) and only then re-enables foreign keys.
      The connection MUST be exclusive during this migration (no concurrent
      readers/writers), so no other work observes the transient OFF window.

      Validation (`foreign_key_check` and behavior probe) runs INSIDE the
      transaction before `COMMIT`. If either check fails, `ROLLBACK` restores
      the original table, the deferred restoration re-enables foreign keys, and
      the migration aborts — no committed broken state and no connection left
      with enforcement OFF. This eliminates both the crash window where a
      committed database lacks its dependent objects or constraints AND the
      connection-state bug where a rollback leaves FK enforcement disabled.

   d. **Interrupted-migration detection:** if `orbits_new` already exists at
      startup (crash between `CREATE TABLE orbits_new` and `COMMIT`), the
      migration detects this intermediate state. If `orbits` still exists:
      drop `orbits_new` and restart the migration from step 1 (re-capturing
      dependent objects, which are still intact). If `orbits` does not exist
      but `orbits_new` does: since transactional SQLite DDL should roll back
      on a process crash, this state is evidence of an unexplained non-
      transactional or manual intervention. Treat as **fatal** — abort startup
      with an error describing the intermediate state. Do not attempt to
      rename `orbits_new` or recreate dependent objects from memory, because
      owned indexes and triggers were destroyed by `DROP TABLE orbits` and
      their DDL was captured only in the memory of a prior process; no durable
      migration journal contains them. The operator must manually inspect and
      repair the database. Before this fatal abort returns, the deferred
      foreign-key restoration (above) still runs, so the connection is not left
      with enforcement OFF.

   Any other `ALTER TABLE` error (lock, corruption, syntax, I/O) MUST be
   treated as fatal — log and abort startup. The implementation distinguishes
   the "duplicate column" case by checking the error message string (SQLite
   provides no structured error code for it) or by querying
   `PRAGMA table_info(orbits)` and skipping the `ALTER TABLE` when a `status`
   column is already present. When the column is already present, the behavior
   probe (not the `ALTER TABLE` result) determines whether the CHECK constraint
   is effective.

**Test requirements for unconstrained column:**
- **Invalid data detection:** create a fixture with
  `ALTER TABLE orbits ADD COLUMN status TEXT NOT NULL DEFAULT 'active'` (no
  CHECK), insert a row with `status = 'bogus'`, run the migration, and verify
  startup aborts with a fatal error naming the invalid rows.
- **Clean rebuild:** fixture with unconstrained column and all valid values
  (`'active'`/`'disabled'` only). Verify rebuild adds the CHECK constraint
  and a subsequent `status = 'bogus'` write is rejected (via behavior probe).
- **Alternate whitespace / misleading comment:** fixture where
  `sqlite_master.sql` contains `CHECK(status IN ('active', 'disabled'))` as a
  SQL comment or with different whitespace formatting. Verify the behavior
  probe (not substring search) correctly determines constraint presence.
- **Equivalent constraint:** fixture with
  `CHECK(status = 'active' OR status = 'disabled')` — semantically equivalent
  but textually different. Verify the behavior probe correctly detects this as
  constrained (rejects `'bogus'`).
- **Dependent objects (conservative all-object rebuild):** fixture with an
  index owned by `orbits` (auto-dropped by `DROP TABLE`), a trigger owned by
  `orbits`, a view referencing `orbits` (NOT auto-dropped — dropped and
  recreated), a view that does NOT reference `orbits`, an external trigger on
  another table whose body references `orbits`, and an external trigger whose
  body does NOT reference `orbits`. Verify ALL six survive the rebuild
  (recreated inside the transaction) with no "already exists" error. Verify the
  step-1 query selects `type` and `tbl_name` and that the drop/recreate set is
  built from those columns only — assert the code performs no `LIKE`/substring/
  token scan of any `sql` body.
- **No `sql`-body scanning:** fixture with a view whose `sql` body contains the
  word "orbits" only inside a string literal or comment, AND a view that
  references `orbits` through a quoted/schema-qualified identifier or a CTE.
  Verify both are dropped and recreated identically (because every user view is
  captured unconditionally), so neither a false positive nor a false negative
  can occur and no post-rebuild "view already exists" or dangling-view error
  arises.
- **Foreign-key enforcement restored on every exit:** using the exact
  migration function, assert `PRAGMA foreign_keys == 1` after each of these
  exits: (1) successful `COMMIT`; (2) `ROLLBACK` from an injected
  `foreign_key_check` violation; (3) `ROLLBACK` from an injected behavior-probe
  failure (rebuilt CHECK missing); (4) an injected SQL error at an arbitrary
  statement mid-transaction; (5) an injected panic/exception during the
  transaction (defer runs on unwind); (6) the interrupted-migration
  intermediate-state fatal abort (step d). Each case first confirms the caller
  entered with `foreign_keys == 1`. The reproducible OFF-after-ROLLBACK control
  is `.research/root-checks/recovery-r14-foreign-keys.sql`.
- **Interrupted migration — `orbits` present:** fixture with both `orbits` and
  `orbits_new` present. Verify startup drops `orbits_new` and restarts the
  migration cleanly (dependent objects are still intact on `orbits`).
- **Interrupted migration — `orbits` absent:** fixture with only `orbits_new`
  present (no `orbits`). Verify startup aborts with a fatal error describing
  the intermediate state (dependent objects lost, no durable migration journal
  to recover them).
- **FK preservation:** test with `memberships REFERENCES orbits(id)` present.
  Verify the create-new → copy → drop-old → rename-new sequence does not
  rewrite child FK declarations. Run `PRAGMA foreign_key_check` after rebuild
  and verify zero violations.
- **Empty table:** fixture with zero rows. Verify the INSERT behavior probe
  works correctly and is rolled back.

#### 17.2 `PRAGMA foreign_keys = ON`

SQLite foreign keys are disabled by default. The current DSN
(`?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`) does not enable them.

**Requirement:** add `_pragma=foreign_keys(ON)` to the DSN. This affects every
connection. FK constraints declared on additive tables are then enforced.
Existing tables (`members`, `slots`, `orbits`, `elements`, etc.) have no
`REFERENCES` clauses in their DDL and are unaffected.

Additive tables that declare FK references:
- `installation_credentials.actor_id REFERENCES actors(id)`
- `memberships.actor_id REFERENCES actors(id)`
- `memberships.orbit_id REFERENCES orbits(id)`
- `telegram_link_codes.orbit_id REFERENCES orbits(id)`
- `telegram_link_codes.issuer_actor_id REFERENCES actors(id)`
- `telegram_link_codes.consuming_actor_id REFERENCES actors(id)`

**Test requirement:** tests must prove that FK violations on additive tables are
rejected (e.g., inserting an `installation_credentials` row with a nonexistent
`actor_id` fails). Validate the migration against handcrafted legacy databases
to ensure existing referential inconsistencies in legacy tables do not break on
database open.

#### 17.3 Credential Provisioning for Backfilled Installations

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

#### 17.4 No Node-Token Control Escalation

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
authorized provisioning issues a control token and recovery material. The
actor's role informs which provisioning is appropriate but does not by itself
grant any control capability.

**Negative test requirement:** a test MUST verify that node-token-only
authentication cannot provision control/recovery credentials. This applies to
all API endpoints and onboarding flows.

#### 17.5 Uniqueness Constraints and Orbit-Alignment Invariant

| Constraint | Mechanism | Violation handling |
|---|---|---|
| One actor per (kind, external_ref) | `CREATE UNIQUE INDEX actors_identity ON actors(kind, external_ref)` | Reuse existing actor (reactivate if left). Covers both `telegram_user` and `app_installation` kinds. |
| One installation actor per slot reference | `UNIQUE(slot_orbit_id, slot_name) ON installation_credentials WHERE slot_orbit_id IS NOT NULL` | Reject duplicate; each active slot maps to exactly one credential row |
| Unique recovery lookup handle | `UNIQUE(recovery_id) ON installation_credentials WHERE recovery_id IS NOT NULL` | Generate-and-retry on collision (astronomically unlikely) |
| Unique code lookup hash | `UNIQUE INDEX ON telegram_link_codes(code_hash)` | Generate-and-retry on collision |
| At most one active membership per actor (Phase 1) | `CREATE UNIQUE INDEX memberships_one_active ON memberships(actor_id) WHERE left_at IS NULL` | Database-enforced; INSERT fails. Telegram consume: ROLLBACK restores code; return `already_linked_same_orbit` or `telegram_member_of_other_orbit` depending on orbit. Recovery: N/A (recovery does not create memberships). |
| One orbit per Telegram user (Phase 1) | Same partial unique index `memberships_one_active` (since Phase 1 has one membership per actor, and each Telegram user maps to one actor, this is sufficient) | Same rejection as above |
| Unique control token hash | `UNIQUE INDEX ON installation_credentials(control_token_hash) WHERE control_token_hash IS NOT NULL` | Enables O(1) token lookup; collision astronomically unlikely (256-bit random) |

**Database-enforced uniqueness (not application-only):** the `memberships_one_active`
partial unique index replaces the prior application-level check. An
application-level check alone is race-prone: two concurrent transactions can
both pass the check and both insert. The database index makes the INSERT itself
fail atomically, ensuring the ROLLBACK in §11 restores the code to unconsumed.
Under `BEGIN IMMEDIATE` serialization, the partial unique index serves as
defense in depth — the step 8 membership check normally catches conflicts first.

**Orbit-alignment invariant:** an `app_installation` actor's active membership
MUST be in exactly the orbit referenced by its
`installation_credentials.slot_orbit_id`. All auth, recovery, rotation, and
issuance queries MUST join the credential's `slot_orbit_id` to the membership's
`orbit_id` (or equivalently verify they match). Migration/reconciliation MUST
fail closed if a mismatch is detected: log a fatal-level error, disable the
affected actor, and refuse to serve until corrected. This invariant prevents
a credential bound to orbit 1 from exercising authority in orbit 2.

#### 17.6 Deriving Installation Actor and Role from `paired_by`

During backfill, each `slots` row is mapped to an `app_installation` actor:
- `actors.kind = 'app_installation'`
- `actors.external_ref` = **generation-scoped identity** (see below)
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
    The separately authorized provisioning (§17.4) must also explicitly
    choose or repair the actor's role — the placeholder `satellite` membership
    does not automatically become control authority upon provisioning.

**Generation-scoped `external_ref` for slot rebind safety:**

The `actors_identity` unique index enforces one actor per
`(kind, external_ref)`. When a slot is rebound (the old coordinator's
`PairSlot` does `INSERT OR REPLACE`, changing `token_hash`, `paired_by`, and
`paired_at`), a new actor must be created for the new binding without violating
the unique index on the old actor's `external_ref`.

**Binding fingerprint:** the `external_ref` for an `app_installation` actor is:

```
{orbit_id}:{slot}:{binding_fingerprint}
```

Where `binding_fingerprint` is a **domain-separated, versioned, full 64-character
lowercase hex** SHA-256 digest:

```
binding_fingerprint = hex.EncodeToString(
  SHA-256([]byte("barycenter/slot-binding/v1:" + token_hash))
)
```

The domain tag `"barycenter/slot-binding/v1:"` is a fixed ASCII prefix
concatenated with the `token_hash` string before hashing. This ensures:
- The fingerprint is cryptographically distinct from any other SHA-256 usage
  in the system (node-token lookup hashes, recovery-secret hashes, etc.).
- The version tag `v1` permits future changes to the fingerprint scheme with
  a different version prefix and explicit migration.
- The hash input is the canonical `token_hash` string bytes (64 lowercase hex
  characters), not the raw 32-byte token.

This is a non-secret fingerprint of the slot's current credential that:
- Is cryptographically unique per binding (different `token_hash` → different
  fingerprint; 256-bit collision resistance). Truncating to fewer bits (e.g.,
  32 bits / 8 hex characters) would permit collisions that make same-millisecond
  rebinds indistinguishable; the full digest eliminates this risk at no storage
  cost.
- Detects every rebind, including same-millisecond rebinds that produce
  identical `paired_at` values.
- Does not expose the node token (the fingerprint is a domain-separated
  hash-of-hash).
- Is deterministically derivable from the `slots` row for both backfill and
  reconciliation.

**Conflict handling for `(kind, external_ref)`:** on `actors_identity` unique
index conflict during actor creation (backfill or reconciliation), the
implementation MUST NOT blindly reuse the existing actor. A fingerprint match
in `external_ref` alone is insufficient to distinguish true idempotency from
a SHA-256 collision — if the new `external_ref` equals the existing one, the
fingerprints match tautologically. Instead, the implementation MUST compare
an **independent stored binding proof**:
1. Read the existing actor's `id` and look up its `installation_credentials`
   row. Read the stored `binding_token_hash`.
2. Compare the stored `binding_token_hash` with the current
   `slots.token_hash` (the canonical full node-token hash that was used to
   compute the fingerprint).
3. If they match: the binding is truly the same — reuse the existing actor
   (true idempotent backfill). The fingerprint match is confirmed by the
   independent stored preimage.
4. If they differ: the `external_ref` fingerprints collided for different
   `token_hash` values (astronomically unlikely with 256-bit SHA-256, but
   defined for completeness). Fail closed: log a fatal-level error with the
   actor ID, orbit ID, and slot name (never credential hashes — they are
   secret-adjacent material and MUST NOT appear in logs per §13). Abort.
   Do not silently reuse or overwrite.
5. If no `installation_credentials` row exists for the actor (e.g., it was
   deleted during a prior reconciliation but the actor was not fully cleaned
   up): fail closed. The actor exists without a verifiable binding; log and
   abort.

Example: orbit 1, slot "a", `token_hash` =
`"abc123def456789012345678901234567890123456789012345678901234abcd"` →
`SHA-256("barycenter/slot-binding/v1:abc123de...64 chars...") = "e8a1f2b3...64 hex chars..."` →
`external_ref = "1:a:e8a1f2b3...64 hex chars..."`.

**Validation:** the binding fingerprint portion of `external_ref` MUST match
the regex `[0-9a-f]{64}`. The full `external_ref` for app installations MUST
match `^[0-9]+:[a-z]:[0-9a-f]{64}$`.

The **current** (live) installation actor uses this fingerprint. When a rebind
occurs:
- The old actor's `external_ref` already contains the old fingerprint.
- The new actor gets `external_ref` with the new fingerprint.
- No conflict occurs because the fingerprints differ.

**Reconciliation detects rebinds** by comparing:
- `installation_credentials.slot_paired_at` (stored at creation) vs
  `slots.paired_at` (current). If they differ, the slot was rebound.
- The binding fingerprint (full 64-hex domain-separated SHA-256:
  `SHA-256("barycenter/slot-binding/v1:" + token_hash)`) in the actor's
  `external_ref` vs the current fingerprint derived from `slots.token_hash`.
  This catches same-millisecond rebinds that `paired_at` alone would miss.

**Rebind detection test requirement:** inject two slot bindings whose
`token_hash` values share the same first 8 hex characters of their SHA-256
digests but differ in the remaining characters. Verify that the full 64-char
fingerprint detects the rebind. This confirms that the prior 32-bit truncated
design would have failed.

`installation_credentials` references the slot via `(slot_orbit_id, slot_name)`.
`slot_paired_at` is copied from `slots.paired_at` at backfill time and serves
as the primary generation marker for rebind detection (§17.8.2).

**Nullable `slots.paired_at`:** the live schema declares `paired_at INTEGER`
with no `NOT NULL` constraint. A legacy slot row may have `paired_at = NULL`
(e.g., slots created by very early code before `paired_at` was populated, or
rows with `paired_by = 0`). The additive
`installation_credentials.slot_paired_at INTEGER NOT NULL` cannot accept NULL.

**Policy:** when `slots.paired_at IS NULL`, backfill uses the sentinel value
`0` for `slot_paired_at`. The value `0` (Unix epoch in milliseconds) is never
a legitimate pairing timestamp and is distinguishable from any real
`paired_at` value. The binding fingerprint (full 64-hex domain-separated
SHA-256 of `token_hash`) remains the authoritative rebind detector regardless
of `slot_paired_at`; the `paired_at` comparison is only a fast-path early
detection. Reconciliation detects rebinds when EITHER `slot_paired_at` differs
from `slots.paired_at` (with NULL treated as `0` for comparison) OR the
binding fingerprint differs.

**Test requirement:** create a handcrafted legacy database with a `slots` row
where `paired_at IS NULL`. Run backfill. Verify `slot_paired_at = 0` in the
credential row. Run reconciliation a second time and verify no-op. Then rebind
the slot (change `token_hash` and set `paired_at` to a real timestamp). Run
reconciliation. Verify the old actor is revoked and a new actor/credential is
created with the real `slot_paired_at`.

**Backfilled role does not confer usable control authority** until the
separately authorized provisioning (§17.4) completes. A backfilled
`primary` or `companion` with `control_token_hash = NULL` can authenticate
only via the node token's `LookupToken` path for playback, not for
control/admin operations. Authorization = token capability × role policy:
node-only → no control operations, regardless of role.

Backfill is idempotent: if the actor/membership/credential row already exists
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

#### 17.7 Dual-Write Coexistence for Telegram Memberships

New or reactivated Telegram memberships created via link consume (§11)
MUST transactionally keep the legacy `members` table consistent. This ensures
the previous coordinator (feature-flag-off) sees the same role and membership.

**On successful Telegram link consume (§11 step 11):**

```sql
INSERT INTO members (orbit_id, tg_user_id, role, joined_at, display_name)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(orbit_id, tg_user_id) DO UPDATE
  SET role = excluded.role,
      joined_at = excluded.joined_at,
      display_name = excluded.display_name
```

This conflict-safe UPSERT targets the primary key `(orbit_id, tg_user_id)`.
It handles the same-orbit case (reactivation) without deleting foreign-orbit
rows. The legacy `members` table has both a `(orbit_id, tg_user_id)` primary
key and a global unique `members_user(tg_user_id)` index. `INSERT OR REPLACE`
would silently delete a foreign-orbit row on the `members_user` conflict,
violating the "foreign orbit → no consume" promise. The UPSERT only handles
`(orbit_id, tg_user_id)` conflicts; an unexpected `members_user` conflict
causes a constraint error, which triggers `ROLLBACK` — the code is restored
to unconsumed and step 8b's foreign-orbit check should have caught this first.

**On membership leave** (when a Telegram user's membership is set to
`left_at IS NOT NULL`):

```sql
DELETE FROM members WHERE orbit_id = ? AND tg_user_id = ?
```

**On membership role change** (if ever applicable in Phase 1):

```sql
UPDATE members SET role = ? WHERE orbit_id = ? AND tg_user_id = ?
```

Feature-flag-off behavior: the old coordinator reads `members` and `slots`
unchanged. The additive tables (`actors`, `memberships`,
`installation_credentials`, `telegram_link_codes`) are ignored. No mutation
to legacy tables occurs except through the dual-write paths above, which
produce the same data the old coordinator would have created directly.

#### 17.8 Reconciliation After Rollback to Old Coordinator

While `self_service_onboarding` is off (flag disabled or old coordinator
deployed), the legacy `members` AND `slots` tables are the authoritative source
for Telegram membership AND app-installation state respectively. The old
coordinator mutates these tables directly:

| Old coordinator method | Affected legacy tables | Effect |
|---|---|---|
| `CreateOrbit` | `orbits`, `members` | New orbit row, primary Telegram member row. NOTE: live `CreateOrbit` does NOT create a slot (slots are created by separate `PairSlot` calls). |
| `AddMember` | `members` | Insert new member with given role |
| `SetMemberName` | `members` | Update `display_name` |
| `TransferPrimary` | `members` | Old primary → companion, target → primary (two Telegram member role updates) |
| `LeaveOrbit` | `members`, `slots` | Delete member, revoke member's slots, promote if primary left; dissolve if last member |
| `PairSlot` | `slots` (`INSERT OR REPLACE`) | New or rebound slot; changes `token_hash`, `paired_by`, `paired_at` |
| `RevokeSlot` | `slots` | Sets `revoked_at` |
| `DeleteOrbit` | `members`, `slots`, `invites`, `availability`, `links`, `orbits` | Full cascade delete |

During rollback the old coordinator's DSN has no `_pragma=foreign_keys(ON)`,
so FK constraints on additive tables are not enforced. The additive tables may
become stale, inconsistent, or point to deleted parents.

**Reconciliation policy on re-deployment with the flag on:**

At startup, reconciliation runs BEFORE the feature serves any requests or
endpoints. It is idempotent and runs inside a single transaction (or a
sequence of small transactions for large datasets). After reconciliation,
`PRAGMA foreign_key_check` is run and MUST return no violations; if it does,
the feature flag remains off and the coordinator logs a fatal-level error.

**17.8.1 Telegram member reconciliation** (legacy `members` → additive
`memberships` / `actors`):

1. **New member in `members` not in `memberships`:** create the
   `telegram_user` actor (or reuse existing by `external_ref`) and insert
   the membership. Same flow as initial backfill.
2. **Member deleted from `members` but active in `memberships`:** set
   `memberships.left_at = reconciliation_timestamp`. The user left during
   old-coordinator operation. Log as warning.
3. **Role difference:** `members.role` differs from `memberships.role` for the
   same `(orbit_id, tg_user_id)`. Update `memberships.role` to match
   `members.role`. Legacy is authoritative during rollback. Log as warning.
4. **Display name difference:** update `actors.display_name` to match
   `members.display_name`. Log as informational.
5. Reconciliation NEVER deletes or overwrites legacy `members` rows. It only
   modifies additive tables to match.
6. Reconciliation is idempotent: running it multiple times produces the same
   result. The same "no-overwrite" rule from §17.6 does NOT apply here —
   reconciliation explicitly resolves known divergence, whereas backfill's
   no-overwrite prevents drift on re-run when no external mutation occurred.

**17.8.2 Slot reconciliation** (legacy `slots` → additive
`installation_credentials` / `actors` / `memberships`):

**Source-first ordering:** reconciliation MUST process destructive changes
(revocations, rebinds, deletions) before creating new actors/credentials.
This prevents a revoked slot from being re-created as a new active actor on the
next startup pass. The algorithm runs in two phases:

**Phase A — Revoke, delete, and repair** (process existing additive state
against current legacy state):

A0. **Disabled-orbit status-aware authorization** — reconciliation MUST NOT
    revoke legacy slots during ordinary startup merely because an orbit is
    disabled. Revoking a slot permanently destroys node and control bindings
    and prevents re-enable without re-provisioning. Instead, new-code
    authorization paths MUST be orbit-status-aware:

    - **New-code playback/heartbeat/media authorization:** new code replaces
      direct `LookupToken` calls with a status-aware lookup that also checks
      the slot's orbit status:
      ```sql
      SELECT s.orbit_id, s.slot
      FROM slots s
      JOIN orbits o ON o.id = s.orbit_id AND o.status = 'active'
      WHERE s.token_hash = ? AND s.revoked_at IS NULL
      ```
      A disabled orbit's slot token returns `ok=false` without revoking
      the slot. This affects the hub's per-connection token resolution,
      `/media` download auth, node heartbeat handler, and any connection
      setup. The `LookupToken` function itself remains unchanged (used by
      legacy code); new-code callers use the status-aware wrapper. Every
      playback/heartbeat/media caller MUST use this wrapper, not the raw
      `LookupToken`.

    - **Actor context / rotate / issuance (§6, §7, §10):** the existing
      staged query handles disabled orbits non-destructively. Stage 1
      validates the credential and binding (the slot is unrevoked → join
      succeeds); stage 2 checks `orbits.status` and returns
      `403 insufficient_capability` for disabled orbits. The credential
      and control token remain valid.

    - **Recovery consume (§5.2):** step 7 already checks `orbits.status`
      and returns `credential_invalid` for disabled orbits.

    - **Re-enable:** when an orbit's status is set back to `'active'`, all
      existing node tokens, control tokens, and recovery credentials
      immediately work again — no re-provisioning or re-pairing needed.
      This is the primary advantage of non-destructive handling.

    - **Gap-minted tokens:** a slot minted by the old binary's `PairSlot`
      during an emergency rollback gap in a disabled orbit has
      `revoked_at IS NULL`. The status-aware lookup rejects it because
      `orbits.status = 'disabled'`. Phase B skips disabled orbits
      (`orbits.status = 'active'` predicate), so no actor/credential is
      created for the gap-minted slot. On later re-enable, the next
      reconciliation's Phase B creates an actor for it (the orbit is now
      active and the slot has no credential row).

    - **Flag-on mutators:** while `self_service_onboarding` is on, every
      legacy mutator with dual-write (§17.8.4) MUST reject disabled orbits:
      `PairSlot`, `AddMember`, invite consume, and Telegram link consume
      MUST check `orbits.status = 'active'` and return an error for
      disabled orbits. This prevents new tokens/members from being created
      in a disabled orbit while new code is running.

    - **One-way slot projection** belongs exclusively in the pre-old-binary
      rollback runbook (§17.11), not in ordinary startup reconciliation.
      Reconciliation reads legacy tables; it MUST NOT mutate them (§17.8.3).

A1. **Orbit dissolved** — process first because it affects multiple slots:
    handled by §17.8.2 case 5 (below). All additive rows for the missing
    orbit are cleaned up.

A2. **Slot revoked** (`slots.revoked_at IS NOT NULL`) but corresponding
    installation actor NOT revoked:
   - Set `actors.revoked_at = reconciliation_timestamp`.
   - **Delete** the `installation_credentials` row (it references a revoked
     slot; the control credential is no longer usable). This releases the
     `UNIQUE(slot_orbit_id, slot_name)` constraint for potential future slot
     reuse.
   - Set `memberships.left_at = reconciliation_timestamp` for the actor's
     active membership (if any).
   - Log as warning.

A3. **Slot rebound** — detected when EITHER `slots.paired_at` differs from
    `installation_credentials.slot_paired_at` (with `NULL` treated as `0` for
    comparison) OR the binding fingerprint (full 64-hex domain-separated
    SHA-256: `SHA-256("barycenter/slot-binding/v1:" + token_hash)`, §17.6)
    derived from the current `slots.token_hash` does not match the fingerprint
    in the actor's `external_ref`:
   - **Revoke old actor:** set `actors.revoked_at = reconciliation_timestamp`.
   - **Delete old credential:** `DELETE FROM installation_credentials WHERE
     actor_id = ?`. This releases the `UNIQUE(slot_orbit_id, slot_name)`
     constraint, allowing the new credential to be inserted in Phase B.
   - **Set old membership left:** `memberships.left_at = reconciliation_timestamp`.
   - Log as warning with old and new `paired_by`/`paired_at`.
   - The old actor's `external_ref` retains its old fingerprint (it is NOT
     updated). The new actor created in Phase B gets a new `external_ref` with
     the new fingerprint, so no `actors_identity` conflict occurs.

A4. **Role/ownership change** (installation actor's implied role changed because
    `TransferPrimary` or `LeaveOrbit` with promotion altered `members.role` for
    the member that `paired_by` references):
   - Re-derive the installation actor's role from the current state of
     `slots.paired_by` → `members.role` per §17.6.
   - If the derived role differs from `memberships.role`, update the
     membership role. Legacy `members` is authoritative during rollback.
   - Log as warning.
   - Note: the control token hash (if provisioned) remains valid; role change
     alone does not revoke the credential. The authorization matrix (§16)
     determines what the changed role can do.

**Phase B — Create** (process legacy slots that have no corresponding additive
state, only for unrevoked slots):

B1. **New unrevoked slot in an active orbit not in
    `installation_credentials`:** query for `slots` rows WHERE
    `revoked_at IS NULL` AND the slot's orbit has `orbits.status = 'active'`
    that have no matching `installation_credentials` row (by `slot_orbit_id`
    and `slot_name`). For each: create an `app_installation` actor with
    `external_ref` per §17.6 (generation-scoped with full 64-hex
    fingerprint), derive role from `paired_by` per §17.6, and create an
    unprovisioned `installation_credentials` row with
    `slot_paired_at = COALESCE(slots.paired_at, 0)`. The `COALESCE` is
    required because live `slots.paired_at` is nullable (declared as
    `paired_at INTEGER` without `NOT NULL` at
    `coordinator/internal/store/orbits.go:43`) while
    `installation_credentials.slot_paired_at` is `INTEGER NOT NULL`. The
    sentinel `0` is safe because `time.Now().UnixMilli()` returns positive
    values. The same `COALESCE(slots.paired_at, 0)` MUST be used in the
    exact INSERT statement for initial backfill, Phase B creation, flag-on
    `PairSlot` dual-write, and any reconciliation comparison involving
    `slot_paired_at`. Same flow as initial backfill.

    **Revoked slots** (`revoked_at IS NOT NULL`) that have no additive state
    are explicitly skipped — they do not receive a new actor or credential.
    A revoked slot may receive a historical revoked actor for audit purposes,
    but that actor MUST have `revoked_at` set at creation time and MUST NOT
    have an `installation_credentials` row.

    **Slots in disabled orbits** are skipped by the `orbits.status = 'active'`
    predicate. These slots remain unrevoked in the legacy table (reconciliation
    does not modify legacy tables, §17.8.3). The new-code status-aware
    playback lookup (§17.8.2 A0) rejects them without revoking them, preserving
    re-enable capability. On orbit re-enable, the next reconciliation's Phase B
    creates actors/credentials for these now-active-orbit slots.

**Orbit dissolution** (handled in Phase A before per-slot processing):

5. **Orbit dissolved** (`orbits.id` no longer exists — the old coordinator
   deleted it via `DeleteOrbit`):
   - All `members` and `slots` rows for that orbit are already deleted by the
     old coordinator's cascade.
   - Additive rows may still reference the deleted `orbits.id`. Clean up in
     child-first FK-safe order:
     a. Revoke all installation actors whose `installation_credentials` row
        references `slot_orbit_id` = the deleted orbit:
        `actors.revoked_at = reconciliation_timestamp`.
     b. **Delete** `installation_credentials` rows for that `slot_orbit_id`.
     c. **Delete** all `memberships` rows for that `orbit_id` (not just
        `left_at` — the orbit no longer exists and FK enforcement requires no
        dangling references).
     d. **Delete** `telegram_link_codes` for that `orbit_id`.
   - After cleanup, `PRAGMA foreign_key_check` confirms no dangling references.
   - Log as warning.

**Idempotency requirement:** run reconciliation twice after a revoke-slot
scenario and prove the second run is a true no-op — no extra actor, no extra
membership, no extra credential row is created. The Phase A-first ordering
ensures that a revoked slot's credential is deleted before Phase B's "no
matching credential" query runs; Phase B then correctly skips the revoked slot
instead of creating a new active actor for it.

**17.8.3 Reconciliation NEVER modifies legacy tables.** It only reads them and
modifies additive tables to match.

**Authority model:**

- While `self_service_onboarding` is off: `members` AND `slots` are
  authoritative for membership AND app-installation state. Additive tables may
  be stale.
- While `self_service_onboarding` is on: all mutations go through the new code
  with transactional dual-write (§17.7 and §17.8.4). Both table sets are
  consistent. Additive tables are the primary operational source; legacy tables
  are kept consistent for rollback safety.
- **Serving gate:** the feature flag endpoints MUST NOT serve until
  reconciliation completes AND `PRAGMA foreign_key_check` returns zero rows.

**17.8.4 Dual-write for legacy mutations while flag is on:**

When the feature flag is on, the following existing store methods MUST
atomically dual-write to additive tables within the same transaction:

| Legacy method | Additional additive writes |
|---|---|
| `CreateOrbit` | Insert a `telegram_user` actor for the creator (or reuse) and a primary membership in the new orbit. No slot/credential write (live `CreateOrbit` does not create a slot; slot creation happens via separate `PairSlot`). |
| `AddMember` | Guard: reject if `orbits.status != 'active'` (§17.8.2 A0). Create a `telegram_user` actor (or reuse existing by `external_ref`) and insert a membership with the given role. |
| `SetMemberName` | Update `actors.display_name` for the `telegram_user` actor identified by `external_ref = "{tg_user_id}"`. |
| `TransferPrimary` | Update `memberships.role` for BOTH the old primary's and new primary's Telegram actors. Additionally update `memberships.role` for any installation actors whose `paired_by` matches the affected members (deriving role per §17.6 rules). |
| `LeaveOrbit` | Set `memberships.left_at` for the leaving member's Telegram actor. Revoke (`actors.revoked_at`) installation actors for the member's revoked slots; delete their `installation_credentials` rows; set their `memberships.left_at`. If promotion occurs: update the promoted member's Telegram actor membership role AND update any installation actors whose `paired_by` matches the promoted member. |
| `RevokeSlot` | Set `actors.revoked_at` for the slot's installation actor. Delete its `installation_credentials` row. Set its `memberships.left_at`. |
| `PairSlot` | Guard: reject if `orbits.status != 'active'` (§17.8.2 A0). If new slot: create `app_installation` actor with generation-scoped `external_ref` (§17.6), membership, and unprovisioned credential. If rebound (existing `(orbit_id, slot)` with different `token_hash`): revoke old actor, delete old credential, set old membership left; create new actor + credential + membership (same as reconciliation case 3). |
| `DeleteOrbit` | Delete additive rows in FK-safe order BEFORE legacy rows (see below). |

**Create Barycenter service-level transaction** (the spec-required orbit
creation operation): the full "create orbit" operation in the new app is NOT
identical to the legacy `CreateOrbit` method. The new operation wraps in one
`BEGIN IMMEDIATE` transaction:
1. Insert `orbits` row.
2. Create `app_installation` actor (creator's first slot).
3. Create membership (role = primary) for the installation actor.
4. Insert `slots` row (the first slot).
5. Insert `installation_credentials` row with control token hash and recovery
   material.
6. Insert `telegram_user` actor for the Telegram owner (if applicable) and
   primary membership.
7. Write legacy `members` row for the Telegram owner (dual-write).
8. Audit.

This is a NEW service method, not a modification of the existing `CreateOrbit`.
The existing `CreateOrbit` continues to work for the legacy Telegram-only flow
with the dual-write additions above.

**`DeleteOrbit` with FK enforcement** — deletion order inside the transaction:

```sql
-- 1. Revoke installation actors whose credentials are about to be deleted
UPDATE actors SET revoked_at = ?
  WHERE id IN (SELECT actor_id FROM installation_credentials WHERE slot_orbit_id = ?)
    AND revoked_at IS NULL;
-- 2. Delete additive children before parents
DELETE FROM telegram_link_codes WHERE orbit_id = ?;
DELETE FROM installation_credentials WHERE slot_orbit_id = ?;
DELETE FROM memberships WHERE orbit_id = ?;
-- 3. Existing legacy cascade (unchanged)
DELETE FROM members WHERE orbit_id = ?;
DELETE FROM slots WHERE orbit_id = ?;
DELETE FROM invites WHERE orbit_id = ?;
DELETE FROM availability WHERE orbit_id = ?;
DELETE FROM links WHERE orbit_a = ? OR orbit_b = ?;
DELETE FROM orbits WHERE id = ?;
```

Telegram actors survive orbit dissolution (they are not orbit-scoped and may
be reused if the user joins another orbit). Only installation actors are
revoked because they are tied to specific slots.

**Test requirement — full rollback cycle including all mutations:**

1. Deploy new code (flag on). Create memberships via Telegram link consume.
   Provision an installation with control credential.
2. Roll back to old code (flag off).
3. Old code performs ALL mutation types:
   a. Add a member (`AddMember` via `/share` consume).
   b. Set member display name (`SetMemberName` via any command).
   c. Remove a member (leave via `/leave` — delete from `members`).
   d. Change a member's role (via `/make_primary` — `TransferPrimary`).
   e. Pair a new slot (`PairSlot`).
   f. Revoke a slot (`RevokeSlot`).
   g. Rebind an existing slot (`PairSlot` with `INSERT OR REPLACE` on an
      existing `(orbit_id, slot)` — different `paired_by`, `paired_at`, and
      `token_hash`).
   h. Dissolve an orbit (`DeleteOrbit`).
4. Re-deploy new code (flag on). Reconciliation runs at startup.
5. Verify:
   - New member exists in `memberships` with correct role.
   - Display name change is reflected in `actors`.
   - Removed member has `left_at IS NOT NULL`.
   - Role and ownership changes are reflected.
   - New slot has an actor + credential row.
   - Revoked slot's actor is revoked; credential is deleted.
   - Rebound slot: old actor is revoked, old credential deleted; new actor
     exists with new `external_ref` fingerprint.
   - Dissolved orbit: all additive rows cleaned up (memberships DELETED, not
     just `left_at`); `PRAGMA foreign_key_check` returns zero rows.
   - Flag-on tests for each legacy method: verify both legacy and additive
     views immediately after each mutation.

This test exercises the old-schema → open-new → open-old → mutate → open-new
sequence across the full old-binary authority surface.

#### 17.9 Executable DDL

The exact idempotent SQL for all additive tables and columns. The schema task
MUST use these statements (or their exact semantic equivalent). Timestamp units
are Unix milliseconds (matching `orbits.created_at`, `slots.paired_at`, etc.).

```sql
-- Additive orbits.status column with CHECK constraint.
-- SQLite accepts ADD COLUMN with CHECK. If column already exists: ignore
-- the specific "duplicate column name" error only; other errors are fatal.
ALTER TABLE orbits ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
  CHECK(status IN ('active', 'disabled'));

-- Actor identity (no secrets).
CREATE TABLE IF NOT EXISTS actors (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL CHECK(kind IN ('app_installation', 'telegram_user')),
  display_name TEXT NOT NULL DEFAULT '',
  external_ref TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  revoked_at INTEGER
);
-- One actor per (kind, external_ref). Covers both actor kinds.
CREATE UNIQUE INDEX IF NOT EXISTS actors_identity
  ON actors(kind, external_ref);

-- Membership role binding.
CREATE TABLE IF NOT EXISTS memberships (
  orbit_id INTEGER NOT NULL REFERENCES orbits(id),
  actor_id INTEGER NOT NULL REFERENCES actors(id),
  role TEXT NOT NULL CHECK(role IN ('primary', 'companion', 'satellite')),
  joined_at INTEGER NOT NULL,
  left_at INTEGER,
  PRIMARY KEY (orbit_id, actor_id)
);
-- At most one active membership per actor (Phase 1).
CREATE UNIQUE INDEX IF NOT EXISTS memberships_one_active
  ON memberships(actor_id) WHERE left_at IS NULL;

-- Installation credentials (app_installation actors only).
-- Rows are DELETED on rebind/revoke (not nulled), so slot columns are NOT NULL.
CREATE TABLE IF NOT EXISTS installation_credentials (
  actor_id INTEGER PRIMARY KEY REFERENCES actors(id),
  slot_orbit_id INTEGER NOT NULL,
  slot_name TEXT NOT NULL,
  slot_paired_at INTEGER NOT NULL,
  -- Independent binding proof: the canonical slots.token_hash at bind time.
  -- Used to verify idempotent backfill on (kind, external_ref) collision and
  -- as a binding predicate in endpoint SQL joins. Stored as the same 64-char
  -- lowercase hex TEXT as slots.token_hash (it IS the same value, not a
  -- derivative). This column is immutable after creation; rebind deletes the
  -- old row and creates a new one.
  binding_token_hash TEXT NOT NULL
    CHECK(length(binding_token_hash) = 64
      AND binding_token_hash NOT GLOB '*[^0-9a-f]*'),
  control_token_hash TEXT
    CHECK(control_token_hash IS NULL
      OR (length(control_token_hash) = 64
          AND control_token_hash NOT GLOB '*[^0-9a-f]*')),
  recovery_id TEXT
    CHECK(recovery_id IS NULL
      OR (length(recovery_id) = 36
          AND substr(recovery_id, 1, 4) = 'rec_'
          AND length(substr(recovery_id, 5)) = 32
          AND substr(recovery_id, 5) NOT GLOB '*[^0-9a-f]*')),
  recovery_secret_hash TEXT
    CHECK(recovery_secret_hash IS NULL
      OR (length(recovery_secret_hash) = 64
          AND recovery_secret_hash NOT GLOB '*[^0-9a-f]*')),
  consumed_at INTEGER,
  created_at INTEGER NOT NULL,
  -- Slot reference references the authoritative slots row.
  FOREIGN KEY (slot_orbit_id, slot_name) REFERENCES slots(orbit_id, slot),
  -- One credential per live slot binding.
  UNIQUE(slot_orbit_id, slot_name),
  -- Recovery fields are all-or-none: either all NULL (unprovisioned)
  -- or all non-NULL (provisioned).
  CHECK(
    (recovery_id IS NULL AND recovery_secret_hash IS NULL)
    OR (recovery_id IS NOT NULL AND recovery_secret_hash IS NOT NULL)
  ),
  -- consumed_at requires recovery to be provisioned.
  CHECK(consumed_at IS NULL OR recovery_id IS NOT NULL)
);
-- Unique recovery lookup handle (non-NULL only).
CREATE UNIQUE INDEX IF NOT EXISTS installation_credentials_recovery
  ON installation_credentials(recovery_id) WHERE recovery_id IS NOT NULL;
-- Unique control token hash (non-NULL only; enables O(1) token lookup).
CREATE UNIQUE INDEX IF NOT EXISTS installation_credentials_control
  ON installation_credentials(control_token_hash)
  WHERE control_token_hash IS NOT NULL;

-- Telegram link codes.
CREATE TABLE IF NOT EXISTS telegram_link_codes (
  id INTEGER PRIMARY KEY,
  code_hash TEXT NOT NULL
    CHECK(length(code_hash) = 64
      AND code_hash NOT GLOB '*[^0-9a-f]*'),
  issuer_actor_id INTEGER NOT NULL REFERENCES actors(id),
  orbit_id INTEGER NOT NULL REFERENCES orbits(id),
  desired_role TEXT NOT NULL DEFAULT 'companion'
    CHECK(desired_role IN ('companion', 'satellite')),
  expires_at INTEGER NOT NULL,
  invalidated_at INTEGER,
  consumed_at INTEGER,
  consuming_actor_id INTEGER REFERENCES actors(id),
  created_at INTEGER NOT NULL,
  -- Consumed state consistency: consumed_at and consuming_actor_id are
  -- all-or-none.
  CHECK(
    (consumed_at IS NULL AND consuming_actor_id IS NULL)
    OR (consumed_at IS NOT NULL AND consuming_actor_id IS NOT NULL)
  )
);
-- Unique code hash for O(1) lookup.
CREATE UNIQUE INDEX IF NOT EXISTS telegram_link_codes_hash
  ON telegram_link_codes(code_hash);
```

**Notes on CHECK constraints:**

- `NOT GLOB '*[^0-9a-f]*'` rejects any string containing a character outside
  `[0-9a-f]`. Combined with `length(...) = 64`, this enforces exactly 64
  lowercase hex characters. GLOB is case-sensitive in SQLite.
- `orbits.status CHECK(status IN ('active', 'disabled'))` is enforced by SQLite
  on INSERT and UPDATE. The constraint is added with the column via
  `ALTER TABLE ADD COLUMN`.
- `telegram_link_codes` consumed-state CHECK enforces that `consumed_at` and
  `consuming_actor_id` are either both NULL or both non-NULL. An invalidated
  code (`invalidated_at IS NOT NULL`) may independently coexist with either
  consumed or unconsumed state — a code may be invalidated (by reissuance)
  after consumption, or invalidated before consumption. The `invalidated_at`
  and `consumed_at` are orthogonal: consumption marks who used it; invalidation
  marks that it was superseded.
- FK `REFERENCES` on composite keys (`slots(orbit_id, slot)`) requires that
  the referenced table has a `PRIMARY KEY` or `UNIQUE` index on those columns.
  `slots` has `PRIMARY KEY(orbit_id, slot)`, satisfying this.
- The `UNIQUE(slot_orbit_id, slot_name)` constraint on
  `installation_credentials` enforces one credential per live slot binding.
  Both columns are `NOT NULL` because credential rows are **deleted** (not
  nulled) on rebind/revoke (§17.8.2). After the old row is deleted, the
  constraint is naturally satisfied for the new row inserted in the same
  transaction.
- **Test requirement for NOT NULL:** verify that inserting a row with
  `slot_orbit_id = NULL` or `slot_name = NULL` is rejected by the database.
  Verify that inserting a row with a valid control token hash but NULL slot
  columns is rejected — this prevents a live credential from existing without
  enforceable orbit ownership.

**FK delete behavior:** no `ON DELETE CASCADE` is declared. Deletion ordering
is handled explicitly in `DeleteOrbit` (§17.8.4). This is deliberate: cascades
would silently delete additive rows when the old coordinator (with FKs off)
deletes legacy parents; explicit ordering makes the delete surface auditable.

**Test requirements for CHECK constraints:** for each CHECK, test at least one
accepted row and one rejected row:
- `actors.kind`: accept `'app_installation'`, reject `'unknown'`.
- `memberships.role`: accept `'satellite'`, reject `'admin'`.
- `installation_credentials.control_token_hash`: accept NULL, accept 64 hex
  chars, reject 63 chars, reject uppercase hex.
- `installation_credentials` recovery all-or-none: accept both NULL, accept
  both non-NULL, reject `recovery_id` non-NULL with `recovery_secret_hash` NULL.
- `telegram_link_codes.desired_role`: accept `'companion'`, reject `'primary'`.
- `telegram_link_codes` consumed-state: accept both NULL, accept both non-NULL,
  reject `consumed_at` non-NULL with `consuming_actor_id` NULL.
- `orbits.status`: accept `'active'`, accept `'disabled'`, reject `'bogus'`.

#### 17.10 Go `_txlock=immediate` Mechanism

The live modernc driver (`modernc.org/sqlite v1.53.0`) defaults `sql.Tx` to
deferred transactions (`BEGIN DEFERRED`). Merely writing "BEGIN IMMEDIATE" in
prose is insufficient — the Go `database/sql` API does not expose a way to
specify the transaction type via `db.Begin()` or `db.BeginTx()` without
driver-specific support.

**Frozen implementation:** add `_txlock=immediate` to the DSN:

```
dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate"
```

With this parameter, every `s.db.Begin()` and `s.db.BeginTx(ctx, nil)` call
issues `BEGIN IMMEDIATE`, acquiring the SQLite write lock at the start of the
transaction. modernc v1.53.0 explicitly supports `_txlock` as a DSN parameter.

**Connection pool interaction with `SetMaxOpenConns(1)`:**

The live code sets `db.SetMaxOpenConns(1)`, meaning only one `sql.Conn` exists.
When a `sql.Tx` is active, that connection is held by the Tx. Any operation
through `sql.DB` (as opposed to `sql.Tx`) would attempt to acquire a connection
from the pool, find none available, and block indefinitely (or until context
cancellation). Therefore:

- **All queries within a transaction MUST go through the `sql.Tx` handle**
  (`tx.QueryRow()`, `tx.Exec()`, `tx.Query()`), never through `s.db`.
- This is already the existing code pattern (verified: `TransferPrimary`,
  `LeaveOrbit`, and `DeleteOrbit` all use `tx.Exec()` and `tx.QueryRow()`
  within their transactions).
- Manual `BEGIN IMMEDIATE` via `s.db.Exec("BEGIN IMMEDIATE")` followed by
  `s.db.Query(...)` MUST NOT be used — with `SetMaxOpenConns(1)`, the
  `s.db.Query()` would try to acquire the sole connection (already held by the
  manual BEGIN), causing a deadlock. Always use `s.db.Begin()` + `tx` handle.

**Backward compatibility:**

- Existing transactions (`TransferPrimary`, `LeaveOrbit`, `DeleteOrbit`) were
  previously deferred. With `_txlock=immediate`, they become immediate — this
  is strictly stronger (acquires the write lock sooner) and has no correctness
  impact. The transactions already use `tx.Exec()`/`tx.QueryRow()` exclusively.
- `SetMaxOpenConns(1)` already serializes all database access, so the change
  from deferred to immediate does not alter observable concurrency behavior.

**New DSN parameter summary:**

| Parameter | Value | Purpose |
|---|---|---|
| `_pragma=journal_mode(WAL)` | Existing | WAL mode for concurrent readers |
| `_pragma=busy_timeout(5000)` | Existing | 5s busy wait instead of `SQLITE_BUSY` |
| `_pragma=foreign_keys(ON)` | New (§17.2) | Enable FK enforcement on additive tables |
| `_txlock=immediate` | New | `BEGIN IMMEDIATE` for all transactions |

**Test requirements:**

1. Assert `PRAGMA foreign_keys` returns `1` on a newly opened store.
2. Create two independent `Store` instances on the same database file (each
   with its own `sql.DB` and `MaxOpenConns(1)`). Use them to test writer
   serialization: one holds a transaction while the other blocks until the
   first commits. This verifies `BEGIN IMMEDIATE` behavior across independent
   connections.
3. After migration and reconciliation, run `PRAGMA foreign_key_check` and
   assert it returns zero rows.
4. After migration, run `PRAGMA integrity_check` and assert `ok`.
5. Handcrafted legacy database test: create a database with only the legacy
   schema (no additive tables), populate with legacy data including
   referential inconsistencies in legacy tables (e.g., `slots.paired_by`
   referencing a deleted member). Open with the new code. Verify: additive
   tables are created, backfill succeeds, `PRAGMA foreign_key_check` reports
   no violations on additive tables, and legacy table inconsistencies do NOT
   cause errors (legacy tables have no REFERENCES clauses).

#### 17.11 Operational Rollback Procedure (Fail-Closed)

**Problem:** the old coordinator binary ignores `orbits.status` and
`actors.revoked_at`. Its `LookupToken` authorizes directly from an unrevoked
`slots` row. If a new-code deployment disables an orbit or additively revokes
an actor, rolling back to the old binary makes that orbit's node tokens usable
again during the rollback interval. Startup reconciliation on the next new
deploy does not protect the interval running the old binary.

**Mitigation — security-relevant projections before rollback:**

The old coordinator's `PairSlot` ignores `orbits.status`, counts only unrevoked
slots against `max_pulsars`, and reuses revoked letters via `INSERT OR REPLACE`.
Merely revoking slots is NOT sufficient: a legacy member can immediately mint
a new node token in a disabled orbit by calling `PairSlot` (which the old
binary exposes as an authorized operation). Therefore, projections must also
block the mutation surface.

Before rolling back from new code to old code, the operator MUST:

1. **Revoke legacy slots for disabled orbits:** for each `orbits` row where
   `status = 'disabled'`, set `slots.revoked_at` on all live slots in that
   orbit. The old binary's `LookupToken` checks `revoked_at IS NULL`; revoked
   slots will correctly fail auth during rollback.

2. **Block slot creation for disabled orbits:** set `orbits.max_pulsars = 0`
   for each disabled orbit. The old binary's `PairSlot` checks
   `count < max_pulsars`; with `max_pulsars = 0`, `0 >= 0` is true, and
   `PairSlot` returns `ErrLimit` before entering the slot-finding loop. This
   prevents new node tokens from being minted in disabled orbits even though
   `PairSlot` ignores `orbits.status`.

3. **Block member addition for disabled orbits:** set `orbits.max_members = 0`
   for each disabled orbit. The old binary's `AddMember` checks
   `count >= max_members`; with `max_members = 0`, the check `count >= 0` is
   always true and `AddMember` returns `ErrLimit`. Using the current member
   count instead of zero is NOT sufficient: if an existing member later leaves,
   `count` drops below the saved value and `AddMember` would succeed again.

4. **Burn pending invites for disabled orbits:** set `invites.used_at` (to the
   current timestamp) for all unused invites in disabled orbits. This prevents
   legacy invite consumption from granting any orbit access during rollback.

5. **Revoke legacy slots for additively revoked actors:** for each
   `installation_credentials` row whose `actor_id` has `actors.revoked_at
   IS NOT NULL`, set `slots.revoked_at` on the corresponding legacy slot
   (identified by `slot_orbit_id` and `slot_name`). This projects the
   additive revocation into legacy state.

These projections are **one-way** — they make the old binary enforce the
security-relevant disable/revoke decisions using state the old binary actually
checks (`revoked_at`, `max_pulsars`, `max_members`, `used_at`). They do NOT
damage rollback compatibility because: revoked slots require an explicit
`PairSlot` to reactivate (which is blocked by `max_pulsars = 0`); and burned
invites are already single-use.

**Post-rollback re-enable requires explicit re-pair/re-provision.** The slot
revocations applied during the projection procedure (step 1:
`slots.revoked_at` set on all live slots in disabled orbits) persist after
the old binary runs and after the new code is re-deployed. The status-aware
playback lookup and every endpoint join require `s.revoked_at IS NULL` — a
revoked slot is NOT accepted merely because `orbits.status` is set back to
`'active'`. Re-enabling an orbit restores only status-aware authorization;
the projected slot revocations must be repaired by explicit trusted re-pair
(`PairSlot` which mints a new token and a new binding, or a dedicated
restoration procedure). Phase B does NOT create credentials for revoked slots
(it has an explicit `revoked_at IS NULL` predicate). This means that:

- Slots that the old binary did NOT rebind/re-pair during rollback remain
  revoked after re-enable. Their node tokens no longer authenticate. The
  operator must explicitly re-pair each affected slot.
- Slots that the old binary DID rebind (via `PairSlot` / `INSERT OR REPLACE`
  while `max_pulsars` was restored after an earlier re-deploy) get a fresh
  `token_hash` and `revoked_at = NULL`. Reconciliation Phase B creates
  credentials for these.
- The `max_pulsars`/`max_members` values are restored from the projection
  journal (§17.11 restoration procedure) on re-deploy; this is separate from
  the slot revocations.

The runbook MUST state: "re-enabling a previously projected disabled orbit
requires explicit re-pair for every revoked slot; status-aware lookup alone
does not un-revoke legacy slots."

**Projection journal** — preserving original values for restoration:

Before overwriting `max_pulsars`/`max_members`, the operator tool MUST
durably record the original values in a projection journal table:

```sql
CREATE TABLE IF NOT EXISTS rollback_projections (
  orbit_id INTEGER NOT NULL,
  original_max_pulsars INTEGER NOT NULL,
  original_max_members INTEGER NOT NULL,
  projected_at INTEGER NOT NULL,
  restored_at INTEGER,
  PRIMARY KEY (orbit_id)
);
```

The complete projection procedure runs in one `BEGIN IMMEDIATE` transaction.
It safely handles first run, rerun while pending, and new projection after a
prior restore-and-quota-change cycle:

```sql
BEGIN IMMEDIATE;
-- 1. Retire completed (restored) projection rows for disabled orbits.
--    This makes room for a new pending generation after a restore cycle.
--    On first run or while already pending, this deletes zero rows.
DELETE FROM rollback_projections
  WHERE restored_at IS NOT NULL
    AND orbit_id IN (SELECT id FROM orbits WHERE status = 'disabled');
-- 2. Save originals ONLY for orbits that have no pending projection.
--    NOT IN (... WHERE restored_at IS NULL) skips orbits that already have
--    an active pending row — their originals are preserved.
INSERT INTO rollback_projections
  (orbit_id, original_max_pulsars, original_max_members, projected_at,
   restored_at)
  SELECT id, max_pulsars, max_members, ?, NULL
  FROM orbits
  WHERE status = 'disabled'
    AND id NOT IN (
      SELECT orbit_id FROM rollback_projections WHERE restored_at IS NULL
    );
-- 3. Apply the projection (idempotent — setting 0 to 0 is harmless).
UPDATE orbits SET max_pulsars = 0, max_members = 0
  WHERE status = 'disabled';
-- 4. Revoke slots, burn invites per steps 1, 4, 5.
COMMIT;
```

**Restoration** on re-deployment with new code (startup reconciliation):

```sql
BEGIN IMMEDIATE;
UPDATE orbits SET
  max_pulsars = (SELECT original_max_pulsars FROM rollback_projections rp
                 WHERE rp.orbit_id = orbits.id AND rp.restored_at IS NULL),
  max_members = (SELECT original_max_members FROM rollback_projections rp
                 WHERE rp.orbit_id = orbits.id AND rp.restored_at IS NULL)
  WHERE id IN (SELECT orbit_id FROM rollback_projections WHERE restored_at IS NULL);
UPDATE rollback_projections SET restored_at = ? WHERE restored_at IS NULL;
COMMIT;
```

**Projection state machine:**
- **Unprojected:** no `rollback_projections` row for the orbit. The orbit's
  `max_pulsars`/`max_members` are the product values.
- **Projected (pending):** `rollback_projections` row exists with
  `restored_at IS NULL`. `original_max_*` holds the product values.
  `orbits.max_pulsars = 0`, `orbits.max_members = 0`. Re-running projection
  is a no-op: step 1 deletes zero rows (no restored row); step 2 skips
  (pending row exists); step 3 sets 0 to 0.
- **Restored:** `rollback_projections` row has `restored_at IS NOT NULL`.
  `orbits.max_*` restored to originals. A subsequent projection: step 1
  deletes the restored row; step 2 inserts a new pending row with the current
  (possibly user-changed) quotas; step 3 sets them to zero. This correctly
  handles quota changes between cycles.

**Crash recovery:** the journal table has `restored_at` to distinguish pending
from completed restorations. On restart, if `restored_at IS NULL` entries
exist, restoration re-runs (idempotent — restoring the same values twice is
safe). A process crash after the projection but before the old binary is
deployed leaves the journal intact; re-running projections sees the pending
row and skips re-saving (originals preserved). If the projection table does
not exist on re-deployment (never projected), no restoration is needed.

The old binary ignores the `rollback_projections` table (unknown table, no
reads). The table is additive and does not affect legacy operations.

**Test requirements for projection idempotency:**
- **Project → project → restore:** orbit with `max_pulsars=5, max_members=10`.
  Run projection. Verify `orbits.max_pulsars=0, max_members=0` and journal
  holds `original_max_pulsars=5, original_max_members=10`. Run projection
  again. Verify journal still holds `5/10` (NOT `0/0`). Run restore. Verify
  `orbits.max_pulsars=5, max_members=10`. Original quotas round-trip.
- **Two complete cycles:** project → restore → project → restore. Verify each
  restore produces the correct originals. If the user changes quotas between
  cycles (e.g., `max_pulsars=3` after first restore), the second projection
  saves `3`, not `5`.
- **Crash during projection:** crash after journal INSERT but before UPDATE of
  `orbits.max_*`. On recovery, re-run projection; verify originals preserved
  and orbit quotas are now zero.
- **Crash during restoration:** crash after orbits UPDATE but before journal
  `restored_at` set. On recovery, re-run restoration; verify orbits restored
  and journal marked.

**Fail-closed procedure:**

1. Stop ingress / disable the feature flag (no new auth/consume requests).
2. Run the security projections above (steps 1–5).
3. Deploy the old binary.
4. Verify old binary starts and serves legacy operations.
5. Verify: `LookupToken` fails for projected slots; `PairSlot` fails for
   disabled orbits (returns `ErrLimit`); `AddMember` fails for disabled orbits.
6. Re-enable ingress.

**Acknowledged limitation:** non-security mutations (display name changes,
new memberships created via additive-only flows) are not projected; they may
diverge during rollback. Reconciliation on the next new deploy handles this
(§17.8). The security projections ensure that disabled/revoked states are
never silently re-enabled and that no new node tokens can be minted in
disabled orbits.

**Emergency rollback (without projections):** if the operator must roll back
without time to run projections (e.g., critical bug), the service/tenant for
affected disabled orbits and revoked actors MUST be kept offline (removed from
ingress/tenant routing, or the entire service stopped) until either projections
are applied or new code is re-deployed. Re-enabling ingress without projections
for a disabled orbit is NOT an accepted fail-closed procedure because the old
`PairSlot` can mint new node tokens. The runbook MUST document this: emergency
rollback without projections = keep affected tenants offline.

**Test requirements:**

1. **Full projection test:** new code disables an orbit and revokes an actor;
   run projections; start old binary; verify `LookupToken` fails for projected
   slots.
2. **PairSlot blocked test:** after projections, attempt `PairSlot` in the
   disabled orbit via old binary — verify it returns `ErrLimit` (not a new
   slot). Also test with all existing slots revoked (count=0, max_pulsars=0):
   verify `0 >= 0` returns `ErrLimit`.
3. **AddMember blocked test:** after projections, attempt `AddMember` in the
   disabled orbit — verify it returns `ErrLimit`.
4. **Invite burned test:** verify all invites for the disabled orbit have
   `used_at IS NOT NULL`; `ConsumeInvite` returns 0 for all of them.
5. **Emergency rollback gap test:** skip projections, start old binary, verify
   the gap exists (PairSlot succeeds, new node token authenticates via live
   `LookupToken`), then re-deploy new code and verify reconciliation restores
   correct state. A slot minted in a disabled orbit during the gap is unrevoked
   in `slots` but the orbit is disabled. Reconciliation does NOT revoke the
   legacy slot (§17.8.3). Instead, the new-code status-aware playback lookup
   (§17.8.2 A0) rejects it: the `JOIN orbits ... AND o.status = 'active'`
   returns no rows for the disabled orbit's slot. Phase B skips the slot
   because the orbit is disabled (`orbits.status = 'active'` predicate). No
   actor or credential is created for the gap-minted slot.
   Test that: (a) before re-deploy, raw `LookupToken` succeeds for the gap
   token (the acknowledged gap); (b) after re-deploy, the new-code
   status-aware lookup rejects the gap token (`ok=false`); (c) no additive
   actor/credential was created for the gap slot; (d) `PRAGMA foreign_key_check`
   passes. This exercises the real new-code playback auth path, not a
   standalone helper. Also test: re-enable the orbit → next reconciliation
   creates actor/credential for the gap-minted slot → status-aware lookup
   now succeeds.
6. Re-deploy new code after projections: verify startup reconciliation restores
   `max_pulsars`/`max_members` to their original values from the
   `rollback_projections` journal table, `restored_at` is set on journal rows,
   burned invites are naturally expired or cleaned up, and
   `PRAGMA foreign_key_check` passes. Also test: project → old member leaves →
   new invite → consume/`AddMember` remains blocked (`max_members=0` prevents
   re-entry even after the member count drops).
7. **Projected slot re-enable test:** project a disabled orbit (slots revoked
   via step 1), re-deploy new code (quota restoration), re-enable the orbit
   (`orbits.status = 'active'`). Verify: projected slots remain revoked
   (`slots.revoked_at IS NOT NULL`) — their node tokens do NOT authenticate
   even though the orbit is now active; Phase B does NOT create credentials
   for them (`revoked_at IS NULL` predicate); the operator must explicitly
   re-pair each slot to restore it. Contrast with the emergency gap test (#5
   above): a gap-minted slot has `revoked_at IS NULL` and Phase B DOES create
   credentials for it on re-enable. These are two distinct cases.
8. **Unchanged slot after projection test:** if the old binary did NOT rebind
   a projected slot during rollback, verify the slot remains revoked after
   re-deploy and re-enable. If the old binary DID rebind a slot (via `PairSlot`
   after `max_pulsars` was restored in a prior cycle), verify the rebound slot
   has `revoked_at IS NULL`, a new `token_hash`, and reconciliation correctly
   creates a new actor/credential for it.

---

## Downstream Task Impact

- **Schema task** (`TASK-260712-1bpog0`): use the exact DDL from §17.9. Add
  `actors` table with non-partial unique index
  `actors_identity ON actors(kind, external_ref)` (covers both actor kinds).
  Add `memberships` with partial unique index
  `memberships_one_active ON memberships(actor_id) WHERE left_at IS NULL`
  (database-enforced). Add `installation_credentials` with `slot_orbit_id
  INTEGER NOT NULL`, `slot_name TEXT NOT NULL` (rows are DELETED on
  rebind/revoke, not nulled), `slot_paired_at INTEGER NOT NULL` (generation
  marker for rebind detection, §17.8.2), `binding_token_hash TEXT NOT NULL`
  (immutable independent binding proof — the canonical `slots.token_hash` at
  bind time, §17.6; used in endpoint SQL joins and collision verification),
  CHECK constraints for hex format (including `binding_token_hash`),
  recovery all-or-none, consumed-at consistency, and FK references. Add
  `telegram_link_codes` with consumed-state all-or-none CHECK and unique code
  hash. Add `orbits.status TEXT NOT NULL DEFAULT 'active' CHECK(status IN
  ('active', 'disabled'))` (§17.1) — handle three cases: column absent (ADD
  with CHECK), column present with CHECK (idempotent skip via rolled-back
  behavior probe), column present WITHOUT CHECK (behavior probe detects
  absence, validate values, rebuild table with exact known live schema if
  clean, abort if invalid; capture and recreate dependent objects inside
  rebuild transaction — classify strictly by `type`/`tbl_name`, NOT by any
  `sql`-body scan; drop and recreate ALL user-defined views and ALL external
  triggers unconditionally (no per-object dependency heuristic); restore
  `PRAGMA foreign_keys` via a defer on every exit (COMMIT, ROLLBACK, error,
  panic, fatal abort); `orbits`-absent intermediate state is fatal).
  All hash columns are 64-char lowercase hex TEXT. No
  `invalidated_at` on credentials — single-row rotation model (§7). Add
  `_pragma=foreign_keys(ON)` AND `_txlock=immediate` to DSN (§17.2, §17.10).
  Use `generateSecret(27)` and `hashToken()`. Add idempotent backfill with role
  derivation (orphan → `satellite`) and generation-scoped `external_ref`
  (§17.6 full 64-hex domain-separated binding fingerprint:
  `SHA-256("barycenter/slot-binding/v1:" + token_hash)`). Set
  `binding_token_hash` from `slots.token_hash` at backfill time. On `(kind,
  external_ref)` conflict, verify stored `binding_token_hash` on the existing
  credential matches current `slots.token_hash` (idempotent) or fail closed
  (mismatch — the fingerprint matched but the underlying token_hash differs,
  indicating a SHA-256 collision). Backfill sets `slot_paired_at` from
  `COALESCE(slots.paired_at, 0)` (NULL → sentinel `0`); the same `COALESCE`
  MUST be used in the exact INSERT for initial backfill, Phase B creation,
  flag-on `PairSlot` dual-write, and all reconciliation comparisons involving
  `slot_paired_at`. Revoked slots are skipped (no
  active actor/credential created). Legacy tables remain intact. Add
  source-first reconciliation for BOTH `members` AND `slots` (§17.8.1,
  §17.8.2): Phase A0 status-aware authorization (non-destructive — new-code
  callers use a status-aware playback lookup that joins `orbits.status =
  'active'`, rejecting gap-minted tokens without mutating legacy `slots`),
  then Phase A (revoke/delete for revoked, rebound, and dissolved slots), then
  Phase B (create for unmatched unrevoked slots in active orbits only).
  Telegram member join/leave/role/name. Rebind detection via `slot_paired_at`
  (NULL → sentinel `0`) AND full 64-hex domain-separated binding fingerprint.
  Orbit dissolution: DELETE memberships, not just `left_at`. Serving gate
  invariant: assert every unrevoked slot in an active orbit has a credential
  row. Run reconciliation twice and prove second pass is no-op. Dual-write for
  ALL legacy mutations while flag is on (§17.8.4): `CreateOrbit`, `AddMember`,
  `SetMemberName`, `TransferPrimary`, `LeaveOrbit`, `RevokeSlot`, `PairSlot`,
  `DeleteOrbit` with FK-safe deletion order. `PRAGMA foreign_key_check` after
  reconciliation. Validate against handcrafted legacy databases. Hash test
  vectors from §3. FK violation tests. CHECK constraint tests (including NULL
  slot columns rejected). Orbit-alignment invariant (§17.5). Unconstrained
  `orbits.status` column fixture test (§17.1 — behavior probe, not substring).
  Rollback projection script: save originals in `rollback_projections` journal
  table, then revoke slots, set `max_pulsars=0`, `max_members=0`, burn invites
  for disabled orbits (§17.11). Restoration on re-deploy reads journal.
  `orbits.status` rebuild uses the SQLite-documented safe sequence (create-new →
  copy → drop-old → rename-new), NOT rename-old-first (§17.1).
- **API task** (`TASK-260712-m5264f`): implement `/v1/recovery/consume` with
  explicit `BEGIN IMMEDIATE` transaction (§5.2 — via `_txlock=immediate` DSN,
  §17.10): rate-limit reservation outside; reads, lifecycle, hash, conditional
  write, audit inside. **Orbit-alignment + binding join** (§17.5) in the
  lifecycle check: join `installation_credentials.slot_orbit_id` to
  `memberships.orbit_id` and `orbits.id`; join `slots` with
  `s.revoked_at IS NULL AND s.token_hash = ic.binding_token_hash` to validate
  the slot is live and the binding is current; use the exact SQL from §5.2
  step 7a, §6, §7 step 6, §10 step 6. A revoked slot or stale binding MUST
  fail the join and produce the appropriate error. Implement
  `/v1/actor/context` with **staged query** (§6): stage 1 validates credential
  + actor + binding (`401` on failure); stage 2 evaluates
  membership/role/orbit (`403` on failure). INNER JOIN for node-token auth
  (§6 serving gate invariant: every unrevoked slot in an active orbit has a
  credential row after reconciliation). Include `actor_id` in create/rotate
  recovery material responses (§4). Implement `/v1/recovery/rotate` with
  **staged in-transaction credential validation + lifecycle check** (§7):
  stage 1 validates credential + actor + binding + bearer re-auth (`401`);
  stage 2 evaluates membership/role/orbit (`403`). Implement
  `/v1/telegram-links` with same staged pattern (§10). Atomic rate-limit
  reservation for ALL authenticated, syntactically valid attempts (not only
  successes) — 10 per actor per 60 min for both rotate and issuance (§9).
  Source-IP extraction: direct TCP peer only; spoofable headers rejected
  unless explicitly trusted proxy configured. Uniform error envelope with
  consistent 401/403 semantics: `401` for invalid/stale/revoked credential
  binding; `403 insufficient_capability` for valid token lacking role/orbit
  authority; `403 credential_invalid` for unauthenticated secret consumes.
  Dummy-hash verification. Node tokens cannot provision control/recovery.
- **Telegram adapter task** (`TASK-260712-2xkyot`): consume is an in-process
  service method, not an HTTP endpoint. Derive `telegram_user_id` from
  verified `Update.message.from.id`. Use rollback-safe `BEGIN IMMEDIATE`
  transaction (§11). **Validate issuer authority at consume time** (§11
  step 5): inside the transaction, after code lookup, check issuer's actor
  revocation, issuer's active membership in the exact target orbit, issuer role
  primary/companion, and orbit status. If issuer lost authority after code
  issuance → `credential_invalid`, code NOT consumed. **Same-orbit legacy
  member rejection** (§11 step 8b): if legacy `members` table has the
  consuming Telegram user in the target orbit → `already_linked_same_orbit`,
  code NOT consumed. The code's `desired_role` never overwrites a migrated
  member's role. Check conflicts in BOTH `memberships` and `members`. Code
  reservation (step 9: `WHERE consumed_at IS NULL AND invalidated_at IS NULL
  AND expires_at > ?`), membership creation (step 10), conflict-safe UPSERT
  dual-write (step 11, §17.7). **Feature serving gate:** endpoints do not
  serve until reconciliation completes and `PRAGMA foreign_key_check` passes.
- **Client tasks** (`TASK-260712-2u1w16`, `TASK-260712-47uve0`): implement
  pending-credential protocol (§5.1) with `ever_sent` marker. **Pending record
  scoped by `(canonical_coordinator_origin, actor_id)`** (§5.1.2). `actor_id` is
  a non-secret integer from the recovery export. Canonical origin =
  `{scheme}://{idna_lowercase_host}:{effective_port}`, no path/query/fragment.
  At most one unresolved `ever_sent` candidate per
  `(origin, actor_id)` across all recovery generations. Before generating a new
  pending token for the same scope, probe the existing candidate first — a lone
  `401` from probe NEVER permits auto-delete while `ever_sent` is true. Silent
  overwrite forbidden. Retry recovery with same tuple after `401` probe. Cancel
  requires destructive-abandon warning. **Persistence:** macOS Keychain
  `SecItemUpdate` for atomic `ever_sent` flip; Windows DPAPI + durable write
  sequence (write temp file → `FlushFileBuffers` → close →
  `MoveFileExW(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)` → reopen +
  decrypt + read-back verification); `ReplaceFile` MUST NOT be used
  (`REPLACEFILE_WRITE_THROUGH` is unsupported). The network request MUST NOT
  begin until read-back verification confirms `ever_sent = true`. Generate
  `replacement_control_token` (32 bytes, hex), write to Keychain/DPAPI before
  sending. Windows DPAPI: current-user scope (omit `CRYPTPROTECT_LOCAL_MACHINE`),
  `CRYPTPROTECT_UI_FORBIDDEN`, exact `CreateFileW` parameters (§5.1.2),
  complete write loop with byte-count check, framed blob with
  magic/version/length header, truncation/trailing-data rejection, `LocalFree`
  on every DPAPI output. NEVER auto-delete once `ever_sent` is true. Handle
  probe per §5.1.1 response table. Show recovery material once. Exact alphabet
  regex for input validation.
- **Test task** (`TASK-260712-38qsku`): hash test vector reproduction
  (`hashToken("000...0")` = `60e05bd...`; `hashToken("ABCDEFG...")` =
  `e45d609...`). Legacy `LookupToken` compatibility. **Consume-vs-rotate
  barrier:** both orderings. **Consume-vs-revoke/leave/disable barrier:**
  both orderings for each lifecycle change. **Two-token concurrency.
  Idempotency lifetime after rotation.** **Telegram rollback-safe transaction
  tests:** same-code/two-user (code remains consumed), two-code/same-user
  (second code NOT consumed), expiry-boundary, reissue-vs-consume,
  membership-insert-failure (code restored), dual-write consistency, UPSERT
  safety (no foreign-orbit deletion). **Rotation-vs-revoke barrier.**
  **Stale-token barrier tests (§7, §10):** authenticate A → pause → recovery
  commits B → resume rotate/issue with A → `401`, no mutation. Also:
  authenticate → role change to satellite → `403`.
  **Issuer authority at consume time (§11 step 5):** issue a code →
  revoke issuer → consume → `credential_invalid`, code NOT consumed.
  Same for issuer-leave, issuer-downgrade-to-satellite, orbit-disable.
  Both writer orderings (lifecycle change before vs after consume start — under
  `BEGIN IMMEDIATE` only the first is reachable).
  **Same-orbit legacy member rejection (§11 step 8b):** legacy-only
  same-orbit primary/companion → `already_linked_same_orbit`, code NOT
  consumed, role preserved. Additive-only (no legacy row) → proceed.
  Foreign-orbit → `telegram_member_of_other_orbit`. Role divergence between
  legacy and additive tables → still rejected (legacy is authoritative).
  **Rate-limit barrier tests at last slot (§9):** N-th attempt succeeds,
  (N+1)-th returns `429`. Test with concurrent requests at boundary.
  **Rate-limit ordering (§9):** for link issuance, verify that a `400`
  from invalid `desired_role` does NOT consume a rate-limit slot (validation
  at step 2, before reservation at step 3). Same for rotation body validation.
  **Double-start pending test (§5.1.2):** start recovery, mark `ever_sent`,
  simulate no response, attempt new recovery for same `(origin, actor_id)` →
  client probes first and refuses to overwrite. Probe returns `401` → client
  does NOT auto-delete, retries recovery with same tuple. Different `actor_id`
  → independent. Same `(origin, actor_id)` with different `recovery_id` while
  existing `ever_sent` → resolve existing first. Power-loss test: verify no
  data loss at each write/flush/replace/send edge. Windows-specific: verify
  `CryptProtectData` called WITHOUT `CRYPTPROTECT_LOCAL_MACHINE` and WITH
  `CRYPTPROTECT_UI_FORBIDDEN`; verify `CreateFileW` uses `GENERIC_WRITE`,
  share mode `0`, `CREATE_NEW` for temp and `GENERIC_READ`, share mode `0`,
  `OPEN_EXISTING` for read-back; verify `LocalFree` called on every DPAPI
  output; verify write loop checks total bytes; verify blob framing rejects
  truncated/trailing data; verify `FlushFileBuffers` precedes close;
  verify read-back succeeds before send; verify network request does not
  begin until durable write completes.
  **Full slot reconciliation test (§17.8.2):** rollback cycle with slot
  creation, revocation, rebind (different `token_hash`/`paired_at`), and orbit
  dissolution. Verify old actor revoked, old credential DELETED (not just
  nulled), new actor created with new `external_ref` fingerprint (full 64-hex),
  `foreign_key_check` passes. Dissolved orbit: memberships DELETED (not just
  `left_at`). **Reconciliation idempotency (§17.8.2):** run reconciliation
  twice after a revoke-slot scenario; prove second run is no-op (no extra
  actor/credential). **Source-first ordering:** verify Phase A (revoke/delete)
  runs before Phase B (create); a revoked slot does NOT get a new active
  actor on second reconciliation run.
  **Rebind unique-constraint test:** execute the exact DDL from §17.9, pair a
  slot, rebind it (revoke old actor + delete old credential + create new actor +
  new credential in one transaction), verify no unique constraint violation and
  both old and new credentials resolve correctly (old fails auth, new succeeds).
  **Rebind fingerprint collision test (§17.6):** inject two slot bindings whose
  domain-separated `SHA-256("barycenter/slot-binding/v1:" + token_hash)` values
  share the first 8 hex characters but differ in the remaining 56; verify the
  full 64-char fingerprint detects the rebind (old actor revoked, new actor
  created with different `external_ref`). On `(kind, external_ref)` conflict,
  verify the implementation compares the stored `binding_token_hash` on the
  existing credential against the current `slots.token_hash` — if they match,
  the binding is truly idempotent; if they differ, the implementation fails
  closed. Inject a conflict with equal fingerprint but different stored
  `binding_token_hash`; verify abort.
  **Nullable `paired_at` backfill test (§17.6):** create a fixture with
  `slots.paired_at = NULL`. Execute the actual Phase B INSERT statement (the
  same SQL that Phase B uses at runtime, not a helper approximation) against
  this row. Verify the INSERT succeeds and produces `slot_paired_at = 0` in
  the credential row (the `COALESCE(slots.paired_at, 0)` in the INSERT
  prevents a `NOT NULL constraint failed` error). Reconciliation second-pass
  is no-op. Rebind the slot (set `paired_at` to a real value), reconcile,
  verify old actor revoked and new actor created with real `slot_paired_at`.
  **NOT NULL slot columns test (§17.9):** verify that inserting an
  `installation_credentials` row with `slot_orbit_id = NULL` or
  `slot_name = NULL` is rejected by the database.
  **Slot-revoke endpoint SQL test (§6, §7, §10):** execute the exact normative
  SQL queries from §6 (actor context stage 1), §7 (rotate stage 1), and §10
  (link issuance stage 1) with an active actor, membership, orbit, and
  credential whose referenced slot has `revoked_at = <timestamp>`. All three
  queries MUST return zero rows (the `JOIN slots ... AND s.revoked_at IS NULL`
  fails). Also test: slot missing entirely, same-coordinate rebind (different
  `token_hash` → binding predicate `s.token_hash = ic.binding_token_hash`
  fails even if slot is unrevoked), and membership in a different orbit than
  `slot_orbit_id`.
  **Staged 401/403 test (§6, §7, §10):** table-driven tests for each
  authenticated endpoint covering: missing credential → `401`, revoked slot →
  `401`, same-coordinate rebind (stale binding) → `401`, revoked actor → `401`,
  stale bearer (concurrent recovery) → `401`, left membership → `403`, disabled
  orbit → `403`, satellite role (rotate/issue) → `403`, missing membership →
  `403`, valid primary → `200` (context) / `200` (rotate) / `201` (issue),
  valid companion → `200`/`200`/`201`. Verify that credential-binding failures
  produce `401` and lifecycle failures produce `403` consistently across all
  three endpoints.
  **Node-token context with additive state test (§6):** execute the node-token
  context query with an unrevoked slot that has a backfilled actor/credential.
  Verify correct `actor_id`, `role`, and lifecycle status are returned.
  **Orbit-alignment invariant test:** attempt auth/recovery/rotation/issuance
  where the credential's `slot_orbit_id` differs from the membership's
  `orbit_id` — must fail.
  **Non-destructive orbit disable/re-enable lifecycle test (§17.8.2 A0):**
  create an orbit with a paired slot and provisioned credential. Disable the
  orbit (`orbits.status = 'disabled'`). Verify: (a) the status-aware playback
  lookup returns no rows (403); (b) the context probe returns `403
  insufficient_capability`; (c) `slots.revoked_at` is still NULL (slot was NOT
  mutated); (d) the credential row still exists. Re-enable the orbit
  (`orbits.status = 'active'`). Verify: (e) the same credentials now
  authenticate successfully (200); (f) no new actor or credential was created
  — the original rows are reused.
  **Legacy mutation dual-write tests (§17.8.4):** with flag on, call each
  legacy method (`CreateOrbit`, `AddMember`, `SetMemberName`,
  `TransferPrimary`, `LeaveOrbit`, `RevokeSlot`, `PairSlot`, `DeleteOrbit`)
  and verify both legacy and additive views are consistent immediately after.
  **Disabled-orbit mutator rejection test (§17.8.4):** with flag on, disable
  an orbit, then attempt `PairSlot` and `AddMember` — both must return an
  error without creating any additive rows.
  **Two-store serialization test (§17.10):** two independent `Store`
  instances on same DB; one holds transaction while the other blocks until
  commit.
  **`PRAGMA` assertions:** `foreign_keys` returns 1; `foreign_key_check`
  returns zero rows after migration/reconciliation; `integrity_check` returns
  `ok`.
  **Handcrafted legacy DB test (§17.10):** legacy-only schema with
  referential inconsistencies in `members`/`slots`; verify additive tables
  created, backfill succeeds, no FK errors on additive tables.
  **`DeleteOrbit` FK-safe ordering test:** verify additive rows deleted
  before legacy parents; `foreign_key_check` passes after dissolution.
  **CHECK constraint tests:** one accepted and one rejected row for every CHECK
  (§17.9 test requirements).
  **Projection idempotency test (§17.11):** project orbit with quotas 5/10 →
  verify journal holds 5/10, orbit has 0/0 → project again → verify journal
  still holds 5/10 (NOT 0/0) → restore → verify orbits restored to 5/10.
  Two complete cycles with user quota change between. Crash after journal
  INSERT but before orbit UPDATE; crash after orbit UPDATE but before journal
  `restored_at` set.
  **Rollback safety test (§17.11):** new code disables orbit + revokes actor →
  run security projections (save journal, revoke slots, max_pulsars=0, max_members=0, burn
  invites) → old binary `LookupToken` fails for projected slots, `PairSlot`
  returns `ErrLimit`, `AddMember` returns `ErrLimit`, `ConsumeInvite` returns 0.
  Also: without projections, verify gap exists (PairSlot succeeds in disabled
  orbit), then re-deploy new code and verify reconciliation.
  **Unconstrained `orbits.status` test (§17.1):** fixture with
  `status TEXT NOT NULL DEFAULT 'active'` (no CHECK) and `status = 'bogus'` row;
  verify migration detects invalid data (via behavior probe) and aborts.
  Fixture with all valid values: verify rebuild adds the CHECK constraint
  (behavior probe confirms rejection). Fixture with alternate whitespace or
  misleading comment in schema: verify behavior probe (not substring) correctly
  determines constraint presence. Fixture with equivalent constraint expression:
  verify probe detects as constrained. Fixture with dependent index/trigger:
  verify survival after rebuild. Fixture with `orbits_new` from crashed prior
  attempt: verify startup recovery. Empty table: verify INSERT probe works.
  FK preservation after rebuild: verify `PRAGMA foreign_key_check` passes.
  **Origin canonicalization vectors (§5.1.2):** run all 17+ shared test vectors
  (default ports, IDNA/punycode, case, trailing dots, IPv4, bracketed IPv6,
  loopback, userinfo rejection, path stripping, zone ID rejection) on both
  macOS and Windows. Verify byte-identical canonical origins.
  **Recovery export round-trip (§4, §7):** verify both create and rotate
  responses include `actor_id`. Verify the exported `actor_id` matches the
  authenticated actor. Test that the pending-recovery scope key
  `(canonical_origin, actor_id)` correctly discriminates installations.
  **Leave-after-projection test (§17.11):** project a disabled orbit with
  `max_members=0`, then have an existing member leave; verify `AddMember`
  still returns `ErrLimit` (the count dropped but `max_members=0` blocks).
  Replay, timing indistinguishability, redaction, control-only recovery,
  role-preservation, pending-credential crash-and-retry, `ever_sent` state
  machine, authorization matrix (disabled orbit → `insufficient_capability`;
  satellite → `insufficient_capability`), exact alphabet validation, dummy-hash
  path, bot message deletion, `Cache-Control: no-store`, actor-context probe,
  backfill idempotency, `orbits.status` migration, **ActorContext capability×
  role matrix** (node + any role → node-only; control + primary → all; control
  + companion → issuance; control + satellite → `403`; unprovisioned → no
  control), **negative node-token escalation**.

---

## Answers to Task Questions

| Question | Answer |
|---|---|
| Recovery endpoint path | `POST /v1/recovery/consume` |
| Recovery request shape | `{recovery_id, recovery_secret, replacement_control_token}` |
| Recovery response shape | `{orbit_id, actor_id, role}` — flat, single membership |
| Uniform errors | §8: `credential_invalid` for unauthenticated secret failures; `unauthorized` for bearer token failures; `insufficient_capability` for valid token lacking authority (satellite, left, disabled orbit) |
| Staged 401/403 | All authenticated endpoints (§6 ActorContext, §7 rotate, §10 link issuance) use a two-stage query pattern. Stage 1 (401): validates credential existence, actor identity, and `binding_token_hash` binding (`JOIN slots ... AND s.token_hash = ic.binding_token_hash`). Stage 2 (403): evaluates membership, role, and `orbits.status`. Node-token context uses INNER JOIN on `installation_credentials` with the binding predicate. Separation ensures auth failures never leak authorization state. |
| Rate limits | §9: per-IP (30/15min), per-recovery_id (10/15min), per-actor rotation (10/60min), per-actor link issuance (10/60min), per-telegram_user consume (10/15min) — ALL syntactically valid attempts counted atomically AFTER auth + validation, BEFORE hash/generation work. Source IP = direct TCP peer; spoofable headers rejected unless trusted proxy configured. |
| Secret rotation | Explicit `POST /v1/recovery/rotate` with control token auth, body validation before reservation, explicit `BEGIN IMMEDIATE` transaction with lifecycle re-check AND in-transaction bearer re-authentication (§7 step 8) |
| Control-token revocation | Recovery overwrites the single `control_token_hash` on `installation_credentials`; no history table. Admin revocation via `actors.revoked_at`. |
| One-time display | Recovery secret shown only at create and explicit rotate |
| Nonpersistence | Client MUST NOT silently persist the secret alongside credentials |
| Post-recovery credential state | Control-only reissue via client-supplied token; node preserved in `slots.token_hash` |
| Telegram desired-role | `companion` (default) or `satellite`; `primary` is `400 invalid_request` |
| Code entropy | 27 chars × 30-symbol alphabet via rejection sampling = 132.49 bits |
| Code expiry | Recovery: single-use, no TTL. Telegram link: 15 minutes |
| Same-orbit conflict | `409 already_linked_same_orbit`; code NOT consumed |
| Foreign-orbit conflict | `409 telegram_member_of_other_orbit`; code NOT consumed. Both `memberships` and legacy `members` checked (§11 step 8). |
| Already-linked | Same as same-orbit conflict |
| Concurrent consume | Recovery: `BEGIN IMMEDIATE` serializes writers; exactly one commits, subsequent consumers see consumed state and enter idempotency path (§5.2 step 9). Telegram: `BEGIN IMMEDIATE` serializes; winner commits, code permanently consumed; loser sees committed state, returns `credential_invalid` (§11 same-code/two-user). |
| Lost response | Client probes `GET /v1/actor/context` with pending token; if 200, promotes directly; if 403, also promotes (token valid, context limited); if 401, retries recovery with same tuple |
| Client crash | `ever_sent` marker in Keychain/DPAPI; pending token NEVER auto-deleted once sent; probe endpoint determines validity; any non-success does not delete pending state |
| Telegram caller trust | In-process service method; principal from verified Update via authenticated Bot API transport (TLS long polling + bot token) |
| Unrecoverable loss | Sole installation + unsaved secret = unrecoverable (stated in UI) |
| Revoked/left/disabled | Recovery and link consume fail with generic `credential_invalid` (dummy hash comparison for timing); authenticated endpoints fail with `401` (revoked token) or `403 insufficient_capability` (valid token, no authority). Lifecycle checks are inside `BEGIN IMMEDIATE` transactions; a concurrent revocation/leave/disable that commits before the transaction starts is seen inside the transaction. Both first consume and idempotent replay check lifecycle. |
| Authorization matrix | Primary or companion: both link roles. Satellite/revoked/left/disabled: none. Primary never granted via link. Disabled orbit on authenticated endpoints: `403 insufficient_capability`. Authorization = token capability × role policy (§16). |
| At-rest hash | Unkeyed SHA-256 of canonical string bytes (not hex-decoded), matching existing `hashToken()`. Hex TEXT storage. Test vectors provided. No HMAC key. |
| Identifier types | `orbit_id`: integer. `actor_id`: integer. `recovery_id`: `rec_` + 32 hex. `telegram_user_id`: string. |
| HTTP hygiene | HTTPS required. `Cache-Control: no-store`. Bodies excluded from logs. |
| Schema ownership | `actors` = identity (no secrets). `installation_credentials` = control/recovery hashes + slot reference + `binding_token_hash` (immutable copy of `slots.token_hash` at bind time for independent collision verification, §17.6/§17.9). `slots` = authoritative node token hash (unchanged). |
| Actor context probe | `GET /v1/actor/context` — read-only, accepts node or control token, returns `{orbit_id, actor_id, role}` or `401` or `403 insufficient_capability`. Full probe response table in §5.1.1. |
| Cancel pending recovery | Requires destructive-abandon confirmation if `ever_sent` is true. A lone `401` from probe is NEVER sufficient to auto-delete. Requires `401` probe + `403` recovery retry in same session + explicit user confirmation. |
| Pending state `ever_sent` | Once true, pending credential is never auto-deleted. Terminal conditions: (a) promoted, (b) superseded by confirmed credential, (c) user-confirmed destructive abandon. One-sent-candidate scoped by `(canonical_coordinator_origin, actor_id)`; `recovery_id` stored for generation tracking. At most one unresolved `ever_sent` per `(origin, actor_id)` (§5.1.2). |
| Hash input convention | SHA-256 of the token/secret's canonical string bytes (64 ASCII hex chars for tokens, 27 uppercase chars for secrets). NOT hex-decoded binary. |
| Legacy coexistence | `members` and `slots` tables remain intact. Backfill is idempotent, never deletes/rewrites. Credential columns nullable for unprovisioned. Old-binary rollback requires fail-closed procedure (§17.11). Telegram dual-write via UPSERT (§17.7). While flag is on, ALL legacy mutations dual-write to additive tables (§17.8.4): `CreateOrbit`, `AddMember`, `SetMemberName`, `TransferPrimary`, `LeaveOrbit`, `RevokeSlot`, `PairSlot`, `DeleteOrbit`. `DeleteOrbit` deletes additive rows in FK-safe order before legacy rows. |
| Reconciliation | While flag is off, `members` AND `slots` are authoritative. On re-deploy, idempotent reconciliation syncs additive tables for BOTH Telegram members (§17.8.1) AND slots (§17.8.2): Phase A0 is non-destructive status-aware authorization — new-code callers use a playback lookup that joins `orbits.status = 'active'`, rejecting gap-minted tokens without mutating legacy `slots`; Phase A revokes/deletes stale; Phase B creates from unrevoked slots in active orbits only (`orbits.status = 'active'` predicate). Rebind detected via `slot_paired_at` AND binding fingerprint. Orbit dissolution: memberships DELETED, not just `left_at`. `PRAGMA foreign_key_check` after reconciliation. Serving gate invariant: every unrevoked slot in an active orbit MUST have a credential row after reconciliation completes. Full rollback cycle test including all legacy mutations. |
| Role derivation from `paired_by` | Matches orbit creator's `tg_user_id` → primary. Otherwise → companion. Orphan/inconsistent rows → `satellite` (lowest capability, cannot issue links). Explicit authorized repair required to upgrade. Backfilled role does not confer control authority until separately authorized provisioning (§17.4). |
| Single-winner mechanism | Recovery: `BEGIN IMMEDIATE` + generation-bound `UPDATE ... WHERE recovery_id = ? AND consumed_at IS NULL` + `RowsAffected()`. Telegram: `BEGIN IMMEDIATE` + rollback-safe transaction (§11). Same conditional-write principle as existing invite consume at line 537. |
| Node-token escalation | Forbidden. Node token grants playback/heartbeat/media only. Legacy installations obtain control through a separately authorized flow (device invite, Telegram-owner authorization). Authorization = token capability × role policy. |
| Recovery rotation model | Single-row overwrite inside `BEGIN IMMEDIATE`: rotation atomically replaces `recovery_id`, `recovery_secret_hash`, resets `consumed_at`. No multi-row generation history. Old `recovery_id` permanently invalid. Rotation-vs-consume and rotation-vs-revoke linearized by writer serialization. |
| Consume-vs-rotate | `BEGIN IMMEDIATE` serializes. If rotation commits first, consume finds no row → `credential_invalid`. If consume commits first, rotation overwrites and consumed secret becomes non-replayable. |
| Telegram transaction safety | Rollback-safe: actor creation, code reservation, membership creation, legacy UPSERT dual-write, and audit all in one `BEGIN IMMEDIATE ... COMMIT`. Failure → `ROLLBACK` → code restored to unconsumed. Winner's commit is final; loser sees committed state. |
| Database-enforced uniqueness | `memberships_one_active` partial unique index on `(actor_id) WHERE left_at IS NULL`. Replaces application-level check. Defense in depth under `BEGIN IMMEDIATE` serialization. |
| Backfill authority | Backfilled role is informational until separately authorized provisioning completes. `control_token_hash = NULL` means playback-only via node token. Authorization = token capability × role policy: node-only → no control, regardless of role. |
| `orbits.status` | Additive column `TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'disabled'))` (§17.1). Database-enforced constraint. Constraint detection via rolled-back SAVEPOINT behavior probe (INSERT/UPDATE with `status='bogus'`; CHECK error = present, success = absent), NOT substring parsing of `sqlite_master.sql`. Migration handles three cases: absent (ADD with CHECK), present with CHECK (idempotent skip), present WITHOUT CHECK (validate values, rebuild if clean, abort if invalid). Referenced by all lifecycle rules. Old coordinator ignores it. |
| Foreign keys | `PRAGMA foreign_keys = ON` in DSN (§17.2). Enforced on additive tables only. Existing tables unaffected. Tests prove violations are rejected. |
| Capability × role | §16 matrix. Node token = playback only. Control + primary/companion = full operations. Control + satellite = no issuance. Unprovisioned = no control. |
| Dual-write safety | `INSERT ... ON CONFLICT(orbit_id, tg_user_id) DO UPDATE` (§17.7). Never `INSERT OR REPLACE`. Foreign-orbit mismatch caught at step 8b before dual-write. Unexpected `members_user` conflict → `ROLLBACK`. |
| Transaction boundaries | Recovery consume (§5.2), rotate (§7), link issuance (§10), link consume (§11) all use `BEGIN IMMEDIATE` (via `_txlock=immediate` DSN parameter, §17.10). Rate limits outside; reads, re-auth, lifecycle, writes, audit inside. |
| In-transaction bearer re-auth | Rotate (§7 step 8) and link issuance (§10 step 8) compute `presented_token_hash` outside transaction; inside `BEGIN IMMEDIATE`, constant-time compare against current `control_token_hash`. Stale (concurrently revoked) token → `401 unauthorized`, no mutation. |
| Consume-time issuer authority | Telegram consume (§11 step 5) checks issuer's actor revocation, active membership in target orbit, role primary/companion, and orbit status. If issuer lost authority after code issuance → `credential_invalid`, code NOT consumed. |
| Same-orbit legacy member | §11 step 8b: if legacy `members` has the Telegram user in the target orbit → `already_linked_same_orbit`, code NOT consumed. `desired_role` never overwrites migrated member's role. Repair is via reconciliation, not unauthenticated code. |
| Executable DDL | §17.9: exact idempotent SQL with CHECK constraints including `orbits.status`, consumed-state consistency, non-partial `actors_identity` index, `slot_paired_at`, hex format checks, FK declarations. No `ON DELETE CASCADE` — explicit ordering in `DeleteOrbit`. |
| Go `_txlock=immediate` | §17.10: DSN parameter makes every `s.db.Begin()` use `BEGIN IMMEDIATE`. All queries within Tx must use `tx` handle, never `s.db`. modernc v1.53.0 supports this. Backward compatible with existing transactions. |
| Slot reconciliation | §17.8.2: source-first ordering — Phase A (revoke/delete) before Phase B (create). Revoked slots → revoke actor, DELETE credential. Rebound slots (detect via `slot_paired_at` AND full 64-hex binding fingerprint) → revoke old actor, DELETE old credential; new actor created in Phase B with new `external_ref`. Phase B: only unrevoked slots without existing credentials get new actors. Orbit dissolution → DELETE memberships + credentials in FK order + `foreign_key_check`. Two-pass idempotency: second run is no-op. |
| Full dual-write | §17.8.4: while flag is on, `CreateOrbit`, `AddMember`, `SetMemberName`, `TransferPrimary`, `LeaveOrbit`, `RevokeSlot`, `PairSlot`, `DeleteOrbit` all atomically dual-write to additive tables. |
| Pending double-start | §5.1.2: one-sent-candidate scoped by `(canonical_coordinator_origin, actor_id)`. `actor_id` from recovery export. At most one unresolved `ever_sent` per `(origin, actor_id)` across all recovery generations. `401` probe NEVER permits auto-delete while `ever_sent` is true. New attempt on same `(origin, actor_id)` requires resolving existing candidate first. Silent overwrite forbidden. |
| Source-IP extraction | Direct TCP peer. Spoofable forwarding headers never accepted unless explicitly trusted proxy configured. Phase 1: no trusted proxy. |
| Slot rebind safety | §17.6: generation-scoped `external_ref` = `{orbit_id}:{slot}:{binding_fingerprint}`. Binding fingerprint = full 64-char lowercase hex domain-separated `SHA-256("barycenter/slot-binding/v1:" + token_hash)`. On rebind: old actor retains old fingerprint (revoked), new actor gets new fingerprint → no `actors_identity` conflict. On `(kind, external_ref)` conflict, read existing actor's `installation_credentials.binding_token_hash` and compare with current `slots.token_hash`: match = true idempotent backfill; mismatch = SHA-256 collision, fail closed. Old credential row DELETED (releases `UNIQUE(slot_orbit_id, slot_name)`), new credential inserted. |
| Orbit-alignment invariant | §17.5: active membership's `orbit_id` must equal credential's `slot_orbit_id`. All auth/recovery/rotation/issuance queries join on this. Mismatch → fatal error, actor disabled. |
| Old-binary rollback | §17.11: NOT unconditionally safe. Fail-closed procedure: stop ingress → revoke legacy slots for disabled orbits (`UPDATE slots SET revoked_at = ? WHERE orbit_id IN (SELECT id FROM orbits WHERE status = 'disabled') AND revoked_at IS NULL`) — projects `orbits.status` into legacy `slots.revoked_at` so the old binary correctly rejects tokens via `LookupToken`'s `revoked_at IS NULL` clause → project into legacy state (max_pulsars=0, max_members=0, burn invites) with original values saved in `rollback_projections` journal → deploy old binary → verify PairSlot/AddMember blocked → re-enable. On re-deploy, restore `max_pulsars`/`max_members` from journal. **Slot revocations are one-way:** re-enabling the orbit restores status-aware authorization for unrevoked slots, but projected slot revocations persist — slots revoked during projection remain revoked and require explicit re-pair/re-provision. Phase B does NOT create credentials for revoked slots. The runbook must state this. Emergency rollback without projections: keep affected tenants offline (PairSlot can mint new node tokens in disabled orbits). |
| Rate-limit ordering | §9: uniform for all endpoints: auth → bounded syntax validation → atomic reservation → generation → writer transaction. `400 invalid_request` NEVER touches the limiter. |
| Persistence primitives | macOS: Keychain `SecItemUpdate` for atomic `ever_sent` flip. Windows: DPAPI current-user scope (omit `CRYPTPROTECT_LOCAL_MACHINE`, set `CRYPTPROTECT_UI_FORBIDDEN`); blob framing: magic (4 bytes) + version (1 byte) + payload length (4 bytes LE, max 16 KB); `CreateFileW` with `GENERIC_WRITE`, share mode `0`, `CREATE_NEW` for temp file; `FlushFileBuffers` before close; `MoveFileExW(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)` for atomic replace; read-back: reopen with `GENERIC_READ`, share mode `0`, `OPEN_EXISTING`, decrypt + verify exact length (reject truncation/trailing data); `LocalFree` all DPAPI output buffers. `ReplaceFile` MUST NOT be used (`REPLACEFILE_WRITE_THROUGH` unsupported). Network request MUST NOT begin until read-back confirms `ever_sent=true`. |
