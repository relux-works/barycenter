# TASK-260712-2u1w16 — macOS onboarding client implementation guard R1

This document is a mandatory precondition for the first implementation pass.
The frozen Rev15 contract in
`.task-board/.resources/TASK-260712-3v1k7q/research.md`, the root amendments,
and the task board remain authoritative. Where this guard is stricter, follow
this guard. Do not silently weaken an invariant; stop and report the exact
conflict instead.

## 1. Deliverable and scope boundary

Implement the reusable macOS **NodeCore credential store, onboarding HTTP
client, and recovery state machine** for create, invite issue/consume, context
probe, recovery consume/rotate, and Telegram-link issue.

This task does **not** redesign or wire the production SwiftUI onboarding
window. `TASK-260712-3dqc3l` owns the UI/data binding. Existing pairing startup
must keep working. Expose typed services/outcomes that the future UI can call.

Allowed production scope:

- `node-app/Sources/NodeCore/Credentials.swift`
- `node-app/Sources/NodeCore/Keychain.swift`
- `node-app/Sources/NodeCore/CoordinatorClient.swift` only for URL/error log
  redaction
- new narrowly named files in `node-app/Sources/NodeCore/`
- tests in `node-app/Tests/NodeCoreTests/`
- `node-app/Package.swift` / resolved dependency state only if the IDNA rule
  truly cannot be met with public Apple APIs; see the stop rule below
- one outcome/evidence document under the task resource directory

Do not edit `OnboardingWindow.swift`, `main.swift`, coordinator, Windows,
Telegram, shared protocol, audio, infra, specs, or board internals. Do not
commit, push, reset, checkout, clean, or rewrite unrelated dirty files.

Clean node-app baseline at delegation time (SHA-256):

- `Package.swift` `eb89b24f13526014d51c8b34bc6b1b897dc7aeed13e4017957cee233073bbce1`
- `Credentials.swift` `268fda3495ae691175c014d08bbc6df5a979ba3bef26218bbdf87223b23a5b6c`
- `Keychain.swift` `03838f05020a9517227fb57315633be604eedf3bafac349c26e7bcc51d8b8bb2`
- `CoordinatorClient.swift` `6d33a7d0c1e4296a5369a6f54416e34a8994f3a981df7b454e0a13462b6416e6`
- `OnboardingWindow.swift` `fe97e00ee79b380b253586173b59724dbd826dcd32ab9fe229752561dbf11cd3`
- `CredentialsTests.swift` `e6b10b55f05972d4e1df5290db3b2ebe5a39fed1725c25a6c92cddf1f6aee045`

Recheck the scoped diff before editing. If an allowed file has changed from
this baseline, do not overwrite it: inspect, preserve, and report the overlap.

## 2. Typed credential model and compatibility

Create a versioned credential bundle able to represent:

- exact node capability: `orbit_id`, `slot`, `node_token`, `ws_url`;
- control capability: `actor_id`, `role`, `control_token`;
- non-secret recovery metadata: `recovery_id` and a local explicit-backup
  acknowledgement if useful;
- canonical coordinator origin needed to scope protected recovery state.

Node and control capability must be independently optional so a recovery on a
clean installation can retain control-only state. Never use the node token as
a control bearer. Recovery changes only control credential/context and
recovery metadata; if node state exists, its token, slot, and ws URL remain
byte-for-byte unchanged.

Keep the existing `NodeCredentials` startup interface source-compatible.
`CredentialsStore.load(besideConfig:)` must still return the preserved node
view when one exists. Existing bot `/pair` saves must remain valid and must not
invent control authority.

For create/invite responses, derive the node WebSocket URL deterministically
from the validated coordinator origin: `https -> wss`, `http -> ws`, path
exactly `/ws`, with no userinfo/query/fragment. Do not derive it from a
response body field that the current API does not return.

## 3. Keychain storage and non-destructive migration

Introduce an injectable protected-store abstraction. Production uses Security
framework calls; tests use an isolated in-memory fault-injecting fake. Tests
must not add, update, or delete the developer's real Keychain items.

Long-lived node/control tokens and sent pending replacement tokens exist only
in Keychain. They must not appear in JSON/config/preferences/files/logs/errors,
test failure strings, URLs, telemetry, or crash-friendly descriptions.

Migration sources are the current DP-keychain item, the current login-keychain
fallback item, and legacy `node-credentials.json`. Use a **distinct versioned
destination account** so migration can never delete or overwrite its only
source. Required order:

1. Read and decode the source without changing it.
2. Write the complete destination bundle.
3. Read the destination back through the same production abstraction and
   compare every field exactly.
4. Only after verified equality, remove that specific source. A legacy file is
   removed only after the destination read-back succeeds.

Failure or crash at any boundary leaves at least one intact readable copy.
Migration is idempotent on restart. If DP Keychain returns
`errSecMissingEntitlement`, a distinct versioned login-keychain destination is
allowed; never confuse that destination with the legacy login source. Do not
retain the current destructive "delete then add on any update error" behavior.
An update failure must leave the prior good item intact or return a structural
storage error.

When both destination locations or destination plus legacy sources exist,
resolve only byte-equivalent state automatically. A credential conflict must
fail closed and preserve all copies; do not pick a token silently.

## 4. Canonical coordinator origin

Implement the exact Rev15 canonical-origin algorithm and all shared vectors:
only absolute `http`/`https`; reject userinfo, opaque/malformed URLs, encoded
host characters, non-decimal/non-quad IPv4, IPv6 zone IDs, and bad ports;
UTS46 non-transitional IDNA/STD3/Bidi/Joiner processing; lowercase A-labels;
strip one trailing root dot; RFC5952 IPv6; omit default 80/443; strip
path/query/fragment.

Foundation URL parsing may be used only behind explicit validation and only if
the complete frozen vectors plus negative edge cases pass. Do not write an
ad-hoc Punycode/IDNA implementation. Do not add a third-party dependency
without first documenting why public Apple APIs cannot satisfy the frozen
profile and stopping for root review. A partial approximation is not an
acceptable implementation.

The canonical origin is non-secret. Use a deterministic collision-resistant
scope key for pending items, but store and read-back the full origin and
`actor_id` inside the protected record and verify both.

## 5. HTTP client contract

Use an injectable async transport/session so every request and response can be
tested without live network access. Production permits HTTPS. Plain HTTP is
permitted only for a literal loopback coordinator; never follow a redirect to
plaintext or another origin. Do not automatically retry non-idempotent
mutations. Set JSON content type and Bearer only where required. Secrets are
always in a bounded JSON body/header, never path/query/fragment.

Implement typed calls matching the currently accepted coordinator exactly:

- `POST /v1/onboarding/orbits` -> 201, no Authorization, title plus caller's
  stable `installation_attempt_id`;
- `POST /v1/device-invites` -> 201 with control bearer;
- `POST /v1/device-invites/consume` -> 200, no bearer;
- `GET /v1/actor/context` -> 200/401/403 with supplied node or control bearer;
- `POST /v1/recovery/consume` -> 200, no bearer;
- `POST /v1/recovery/rotate` -> 200 with control bearer and body `{}`;
- `POST /v1/telegram-links` -> 201 with control bearer.

Decode success and the uniform error envelope with bounded response bytes,
strict scalar types, required fields, enum validation, exact status/code
compatibility, and rejection of duplicate/trailing/unknown JSON where the
contract requires it. Errors may expose typed status/code and
`retry_after_seconds`; they must never retain or echo response bodies,
request bodies, bearer values, entered codes, full URLs, or transport error
strings that may contain those values. User-facing descriptions stay generic.

Do not create a Telegram URL containing `link_code`; return the code and bot
username as separate typed values. Do not persist invite/link codes. Existing
`pairNode` and WebSocket logging must be redacted while preserving useful
origin-level diagnostics: never log URL userinfo/path/query/fragment or raw
server bodies.

## 6. Crash-safe recovery state machine

Implement Rev15 section 5.1 verbatim through a serialized service/state
machine. The protected pending record is exactly:

`canonical_coordinator_origin`, `actor_id`, `recovery_id`,
`pending_control_token` (32 CSPRNG bytes, lowercase 64 hex), `ever_sent`.

The user-supplied recovery secret is memory-only and must never be captured by
a persisted closure or protected record. Before the first request:

1. Reject/resolve an existing `ever_sent=true` record for the same
   `(origin, actor_id)`; a different recovery generation does not bypass it.
2. Persist the new exact record with `ever_sent=false`.
3. Atomically update that same Keychain item to `ever_sent=true` via
   `SecItemUpdate`.
4. Read it back and verify every field and `ever_sent=true`.
5. Only then permit the transport's send entry point.

All recovery starts in the process must share serialization sufficient to
prevent two service instances/threads from sending different candidates for
one scope. Duplicate Keychain item races must be re-read and resolved, not
overwritten. Once true, `ever_sent` is monotonic.

Response handling is exact:

- consume 200: atomically promote pending control/context, preserve node state,
  verify, then delete pending;
- consume 400 after any send: keep pending;
- consume 403: keep pending;
- consume 429/5xx/network/cancellation/decoding ambiguity: keep pending;
- restart with sent pending: probe it before generating anything;
- probe 200: promote with returned context, then delete pending;
- probe 403 `insufficient_capability`: token authenticated; promote with limited
  context even though response has no actor/orbit/role, retaining known
  protected metadata and reporting limited context;
- probe 401: keep and retry recovery only with the same tuple/token after the
  user supplies the secret again;
- probe 429/5xx/network: keep and retry/back off; never overwrite;
- probe 401 then retry 403: still keep. Destructive abandon is allowed only
  after the exact explicit warning/confirmation rules in Rev15.

Promotion must be crash-tolerant: a crash between active-bundle save and
pending deletion must restart by detecting that the active control token is
the same candidate, verifying it, and completing cleanup without generating or
sending another token. Storage errors never turn into claimed success.

No cancellation after the send gate may erase or replace pending state.
Cancellation before `ever_sent=true` may remove only an exact unsent candidate.

## 7. One-time recovery material and pasteboard

`recovery_secret` returned by create/rotate is an in-memory-only, deliberately
non-`Codable`, non-`CustomStringConvertible` sensitive value. It is never
silently saved. Explicit export/copy must package all three values together:
`actor_id`, `recovery_id`, `recovery_secret`.

Expose a testable explicit export helper and a pasteboard lease abstraction for
the future UI. Never copy automatically. A system pasteboard implementation
must clear after a bounded TTL only if both its change count and exact exported
payload still match the lease; it must never erase newer user clipboard data.
Provide explicit early clear. Do not put raw secrets in timer labels, closure
descriptions, logs, errors, notifications, pasteboard item metadata, or file
names. An explicit user-selected save returns/writes the full export only to
that selected destination; do not create sidecar, autosave, recent-document,
or temp copies.

The UI-facing model must carry the exact warning from Rev15 in RU and EN (or a
stable localization key plus tested exact fallback): loss of the sole
installation plus an unsaved recovery secret is **unrecoverable**. Do not mark
the material backed up merely because it was displayed or copied; only an
explicit user acknowledgement may set the non-secret flag.

## 8. Required deterministic tests

At minimum, add focused tests proving:

1. Versioned bundle encode/decode and node/control split; no secret-bearing
   type accidentally conforms to printable/encodable persistence protocols.
2. Legacy DP, login-keychain, and file migrations; read-back-before-delete;
   every write/read/delete crash point; conflict fail-closed; idempotent restart;
   exact preservation of token/slot/ws URL.
3. Existing `CredentialsStore.save/load` pair compatibility and a control-only
   recovered bundle not masquerading as node credentials.
4. Every canonical-origin vector and additional malformed, userinfo, Unicode,
   encoded-host, IPv4, IPv6-zone, default-port, and cross-origin redirect cases.
5. Exact request method/path/auth/body and success/error decoding for all seven
   endpoints; maximum size, duplicate/trailing/unknown JSON, wrong scalar,
   redirect, plaintext, timeout, cancellation, and response-body redaction.
6. Recovery barriers with an instrumented transport: zero sends before verified
   `ever_sent=true`; update/read-back failure; crash after false write, after
   true update, after read-back, during send, after server success, during
   active promotion, and before pending deletion.
7. Full consume/probe matrix, including 403 limited promotion, 401 same-token
   retry, 401->403 retention, 400-after-send retention, 429 retry metadata,
   network/5xx/decoder ambiguity, cancellation, and explicit destructive
   abandon.
8. Concurrent double-start from multiple service instances: exactly one token
   can cross the send boundary for a scope; different `(origin, actor)` scopes
   remain independent; recovery generations cannot overwrite a sent candidate.
9. Recovery changes control only and preserves existing node bytes. Promotion
   crash/restart converges without a duplicate request.
10. Recovery export contains all three fields only on explicit action; clipboard
    TTL clears the leased payload, preserves changed clipboard data, explicit
    clear is idempotent, and no automatic copy occurs.
11. Secret canaries do not occur in captured logs, error descriptions, URLs,
    persistence outside the expected fake-Keychain encrypted-value boundary,
    filenames, or ordinary debug/reflection output used by the app.
12. Existing NodeCore tests and production Swift build still pass.

Use injected deterministic CSPRNG/clock/transport/store/pasteboard in tests;
production defaults must use Security framework CSPRNG and real monotonic-safe
timing where appropriate. Do not make timing-only flaky tests.

## 9. Verification and handoff

Run and record exact commands/results:

- focused new Swift tests;
- full `swift test` from `node-app`;
- repeat the recovery/concurrency suite enough to expose races (deterministic
  barriers are still mandatory);
- production `swift build -c release` (and any repository-standard signed-app
  compile check that does not mutate external state);
- `git diff --check` and formatting/typecheck checks;
- scoped secret/URL/log scans with synthetic canaries;
- `git status --short` and SHA-256 for every changed/added file.

Write an outcome resource containing: behavior matrix, migration crash matrix,
recovery response matrix, changed-file hashes, commands with exit status,
remaining risks, and confirmation that real Keychain/user data was untouched.
Do not claim the board task accepted/done. Leave it for root's line-by-line
review, independent security/migration review, and root reruns.

## 10. Mandatory stop conditions

Stop without weakening the design if any of these is true:

- exact UTS46 profile cannot be proven with public existing dependencies;
- the server response lacks data needed for a required safe transition and no
  frozen rule supplies it;
- safe non-destructive migration cannot distinguish source/destination items;
- Keychain semantics cannot enforce read-back-before-send or double-start;
- an existing overlapping edit appears in an allowed file;
- a requested behavior would persist or expose plaintext recovery material.

Report the smallest concrete blocker and proposed options. Do not substitute a
best-effort implementation.
