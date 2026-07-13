# TASK-260712-47uve0 — Windows DPAPI onboarding client guard R1

This is a mandatory first-pass implementation guard. The frozen Rev15 contract
in `.task-board/.resources/TASK-260712-3v1k7q/research.md`, its root amendments,
the accepted coordinator onboarding boundary, and the task card remain
authoritative. Where this guard is stricter, follow it. Do not weaken an
invariant to make a test pass; report the smallest concrete conflict instead.

## 1. Deliverable and ownership boundary

Implement the reusable Windows credential repository, DPAPI/durable-file
primitives, strict onboarding HTTP client, crash-safe recovery service, and
explicit recovery export/clipboard abstractions. Preserve the existing
`Credentials`, `LoadCredentials`, `Credentials.Save`, legacy `/pair`, CLI, and
Win32 onboarding callers as compatible node-only views.

This task does not redesign the Windows onboarding UI. `TASK-260712-2fe5bz`
owns UI/data integration. Expose typed services and a clipboard adapter that a
real non-NULL UI owner HWND/dispatcher can call later. Do not add mock UI claims.

Allowed production scope:

- `pulsar-win/config.go`, `pair.go`, and narrowly necessary compatibility edits
  in `main.go` / `ui.go`;
- new narrowly named portable and `*_windows.go` / `*_nonwindows.go` files at
  the `pulsar-win` module root;
- root-package tests and Windows compile-only tests;
- `go.mod` / `go.sum` only for a reviewed IDNA dependency;
- task outcome/evidence files.

Do not edit `cmd/pulsar-win-probe`, `internal/winprobe`, native capture code,
manifests, `ui_windows.go`, audio/playback/protocol code, coordinator, macOS,
specifications, or sibling task resources. Do not commit, push, reset,
checkout, clean, or rewrite unrelated dirty files.

Frozen scoped baseline (SHA-256):

- `config.go` `ca471c2739007489c6ab91cfaffbe87ddd223d3dbbeffc4a1db1665d7e182d4d`
- `pair.go` `3c68d22a05adba44d7889b22c6c6184bb987c16df25401383a0db86f56c5ded8`
- `main.go` `81473f626f94f28208995a968b2da22ac847033392c06050abd00e4d49836ed8`
- `ui.go` `69933470d7a2c1e88296f7ba62e21e966a5fb70e0df13c63fe8d75b9013d48f2`
- `ui_common.go` `4abe20d7524e6e3b741e31815e22fbd387f5dc8158fc75092bee6aecdfb1c4d0`
- `ui_windows.go` `6ec4135c9b3ff771a7ee15bfaa7950bd3800ed25aff3ea34fe4485aa25c41a61`
- `ui_stub.go` `ae1eba0eb7f022bf55d8262111c7bb86e39fd26f99192fcd435836cad4334759`
- `config_test.go` `5c04033e78ad16bd7cf60d459ccc13835f3894f2c1b506041fc7aed5c4268b44`
- `pair_test.go` `e96eb7b5c3d3a1910d8edcaccf1bd514f71af8f92c38f17b1cccd48722f46844`
- `go.mod` `5a3a43656f1dd8f310e3e61f1e8194b979aa42b8f64fa5386fa5709ffb025977`
- `go.sum` `22e806d87a9b8ae365506c29fd92750d3c4654d768d4cb649eedad86710fbc7`

Re-hash before editing. If an allowed file drifted, inspect and preserve the
overlap. The lifecycle producer owns the untracked probe subtree concurrently.

## 2. Versioned credential model and compatibility

Create a strict versioned bundle with independently optional capabilities:

- node: exact `orbit_id`, one lowercase `slot`, 64-lowercase-hex `node_token`,
  validated `ws_url`;
- control: positive `actor_id`, `orbit_id`, role, 64-lowercase-hex
  `control_token`;
- non-secret `recovery_id`, canonical coordinator origin, and only an explicit
  non-secret backup acknowledgement if needed.

Node and control bearers are never interchangeable. A recovered clean install
may be control-only and must not masquerade as paired node state. Recovery
updates control/context only; existing node token, slot, orbit, and WS URL must
remain byte-for-byte unchanged. Legacy `/pair` and re-pair update only the node
view and must not erase a valid control capability.

Keep the current `Credentials` public shape and startup call sites working.
`LoadCredentials` returns the node view or nil. `Credentials.Save` persists a
node-only update into the protected bundle. It must never write plaintext or
claim success if the protected write/read-back failed. Ordinary and debug
formatting of every token-bearing type must be explicitly redacted, including
`fmt.Stringer` and `fmt.GoStringer` paths.

Create/join response WS URLs are derived from the validated coordinator
origin: `https -> wss`, loopback `http -> ws`, path exactly `/ws`, and no
userinfo/query/fragment. Legacy `/pair` response WS URL is preserved exactly
after strict validation.

## 3. Canonical coordinator origin

Implement the Rev15 shared canonical-origin vectors exactly: absolute
`http`/`https` only; no userinfo, encoded host characters, opaque/malformed
URL, invalid port, non-quad/non-decimal IPv4, IPv6 zone, or host ambiguity;
UTS46 non-transitional IDNA with STD3/Bidi/Joiner validation; lowercase
A-labels; one trailing root dot stripped; RFC5952 IPv6; default 80/443 omitted;
path/query/fragment removed from the canonical origin.

Use `golang.org/x/net/idna` with explicit profile options if needed; do not
write ad-hoc Punycode. Add no other dependency without reporting why. The
pending filename/key uses a collision-resistant digest of
`canonical_origin + NUL + actor_id`, but every protected record stores and
read-back-validates the full origin and actor ID.

## 4. Protected repository and DPAPI invariants

Use an injectable portable repository with separately injectable data
protector, random source, clock, and Win32 file operations. Host tests must use
fakes and must never touch a developer credential/clipboard. The real wrapper
lives behind Windows build tags; non-Windows production defaults return a
clear unsupported-platform error rather than creating a plaintext fallback.

All long-lived node/control tokens and pending replacement tokens are stored
only in current-user-DPAPI ciphertext files. `CryptProtectData` and
`CryptUnprotectData` use `CRYPTPROTECT_UI_FORBIDDEN` and never
`CRYPTPROTECT_LOCAL_MACHINE`. No secret description/optional entropy. Call
`LocalFree` exactly once for every non-NULL encrypt/decrypt output, including
partial allocations on failure. Best-effort zero temporary plaintext byte
slices after copying; never claim Go strings can be securely erased.

Plaintext before encryption is a deterministic strict envelope:

- four-byte magic;
- one-byte version;
- four-byte little-endian payload length;
- at most 16,384 payload bytes;
- exact JSON/schema fields, no duplicate/unknown/trailing content.

Reject truncated header/payload, unknown version, oversized length, and any
trailing byte. Ciphertext files are capped at 1 MiB before allocation.

Every protected write, including active bundle and pending state, uses the
frozen same-volume durable algorithm. For the recovery `ever_sent=true`
transition it is a hard network-send barrier:

1. random same-directory temp path; `CreateFileW(GENERIC_WRITE, share=0,
   CREATE_NEW, FILE_ATTRIBUTE_NORMAL|FILE_FLAG_WRITE_THROUGH)`;
2. complete `WriteFile` loop; success with zero bytes is failure; exact total
   must equal DPAPI ciphertext length;
3. `FlushFileBuffers`;
4. `CloseHandle`, whose failure aborts before move;
5. `MoveFileExW(temp, destination,
   MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH)`; never `ReplaceFileW`
   and never cross-volume copy;
6. reopen with `CreateFileW(GENERIC_READ, share=0, OPEN_EXISTING,
   FILE_ATTRIBUTE_NORMAL)`, `GetFileSizeEx`, bounded complete read,
   `CryptUnprotectData`, strict envelope/schema decode, exact expected-field
   comparison, then successful `CloseHandle`;
7. only after step 6 may a recovery send begin.

On every branch: one close attempt per acquired handle; exact `LocalFree`
ownership; delete a failed temp where possible; never move after flush/close
failure; retain a corrupt destination for fail-closed diagnosis; clean stale
task-owned random temps on a later attempt. A read-handle close failure blocks
the send even after otherwise valid read-back. Errors expose only stable
operation classes, never paths, ciphertext/plaintext, URLs, tokens, or raw
Win32 wrappers that may include them.

Use process-global keyed serialization across repository/service instances.
Where practical, hold a same-scope Windows lock handle with share mode zero for
the full recovery transition so two processes cannot overwrite one candidate;
a crash must release the handle without requiring deletion of a secret-bearing
lock name. Never rely on last-writer-wins rename for double-start safety.

## 5. Non-destructive plaintext migration

The existing `%APPDATA%/Pulsar/credentials.json` is a legacy source only. Use
a distinct versioned encrypted destination name. Migration order is fixed:

1. bounded strict-read and validate legacy node credentials without mutation;
2. if an encrypted destination exists, decrypt/validate it first;
3. only byte-equivalent node state may reconcile automatically; any conflict
   or corrupt protected destination fails closed and preserves both files;
4. otherwise write a complete versioned bundle to the encrypted destination
   via the durable algorithm;
5. reopen/decrypt/compare every field through the production abstraction;
6. only then delete that exact legacy plaintext source.

Crash/delete failure leaves at least one readable copy and converges on
restart. A surviving equivalent plaintext source is retried for deletion. Do
not delete-before-write, overwrite a corrupt destination, silently pick one
credential, or restore plaintext as fallback. Existing node token/slot/WS URL
must be identical after migration. Re-pair must preserve any control state.

## 6. Strict HTTP boundary

Use an injected async/context-aware transport. Production permits HTTPS and
literal loopback HTTP only. Disable automatic redirects; classify same-origin
redirects as errors too rather than replaying mutations. Never retry a
non-idempotent request automatically. Bound request and response bodies (64 KiB
maximum response is sufficient), close bodies on every path, and reject
wrong/duplicate/unknown/trailing JSON, wrong scalar types, missing required
fields, invalid enums, invalid status/code pairs, and excessive nesting.

Implement typed calls for the accepted coordinator routes:

- `POST /v1/onboarding/orbits` -> 201, no bearer;
- `POST /v1/device-invites` -> 201, control bearer;
- `POST /v1/device-invites/consume` -> 200, no bearer;
- `GET /v1/actor/context` -> 200/401/403, supplied node/control bearer;
- `POST /v1/recovery/consume` -> 200, no bearer;
- `POST /v1/recovery/rotate` -> 200, control bearer, exact `{}` body;
- `POST /v1/telegram-links` -> 201, control bearer;
- preserve legacy `POST /pair` through the same safe transport/decoder.

Bearer authority is origin-bound by construction: public authenticated methods
accept a stored typed capability plus matching canonical client origin, not an
arbitrary raw token. Raw bearer-taking primitives remain internal. Codes and
secrets occur only in bounded bodies/headers, never path/query/fragment.

Public errors expose typed status/code/retry metadata and stable generic text.
They never retain/unwrap response/request bodies, bearer, entered code,
recovery secret, full URL, path/query, `url.Error`, redirect target, or raw
transport/filesystem strings. `PairError.Body` must be removed or made
unobservable. Validation must not echo an unsafe WS URL.

Return Telegram link code and bot username separately; never synthesize or log
a URL containing the code. Do not persist invite/link/pair codes.

## 7. Crash-safe recovery state machine

The protected pending record is exactly:

`canonical_coordinator_origin`, `actor_id`, `recovery_id`,
`pending_control_token` (32 CSPRNG bytes, lowercase 64 hex), `ever_sent`.

The user-entered recovery secret is memory-only and absent from every record,
closure description, log, path, and timer. For a new attempt:

1. acquire exact-scope serialization;
2. load/resolve an existing pending record before token generation;
3. persist the exact new candidate with `ever_sent=false` and verify it;
4. durably replace it with the same record plus `ever_sent=true` using §4;
5. reopen/decrypt/verify all fields and successful handle cleanup;
6. only then enter the transport send callback.

Once true, `ever_sent` is monotonic. A different `recovery_id` cannot bypass an
unresolved candidate for `(origin, actor_id)`. Duplicate races re-read and
resolve; no overwrite. Cancellation before the send gate may delete only an
exact unsent candidate. No cancellation after the gate may erase/replace it.

On service construction/startup expose a no-secret resume/probe operation for
sent pending records. It must probe before generating anything or asking for a
secret. Response rules are exact:

- consume 200: durably promote pending token/context into active bundle,
  preserving node bytes, read-back verify, then delete pending;
- consume 400/403/429/5xx/network/cancel/decoder ambiguity: retain pending;
- restart probe 200: promote and delete after verified active write;
- probe 403 `insufficient_capability`: authenticated; promote with limited
  context and retain known protected metadata;
- probe 401: retain and require same tuple/token plus user secret for retry;
- probe 429/5xx/network: retain and back off;
- probe 401 then recovery 403: retain; destructive abandon only after the
  Rev15 warning and explicit confirmation.

A crash after active promotion but before pending deletion converges by
detecting the identical active control token and completing exact deletion
without new generation/send. Storage errors never become claimed success.

## 8. One-time material, direct export, and Windows clipboard

Represent recovery secret and one-time material with unexported secret fields,
no JSON/Text marshal support, and redacted `String`/`GoString`. Explicit reveal
is reachable only by request-body encoding or user-invoked export/copy. Export
contains exactly `actor_id`, `recovery_id`, `recovery_secret`; display alone or
copy does not mark it backed up. Carry exact RU/EN unrecoverable-loss fallback
copy from Rev15.

Direct save writes only to the explicit user-selected destination: no temp,
sidecar, autosave, recent-document entry, secret filename, or log. Failure must
not leave a falsely successful partial export.

Clipboard is explicit-only and requires a real non-NULL owner HWND; do not call
`OpenClipboard(NULL)` because after `EmptyClipboard` that makes
`SetClipboardData` fail. The Windows adapter must:

- run through the owner-window/UI dispatcher;
- open once, empty once, publish `CF_UNICODETEXT` from `GMEM_MOVEABLE` memory,
  and transfer/free handles correctly on every failure path;
- register and publish `ExcludeClipboardContentFromMonitorProcessing`, plus
  DWORD-zero `CanIncludeInClipboardHistory` and `CanUploadToCloudClipboard`;
  inability to install the exclusion markers fails before exposing the secret;
- record only a lease ID, clipboard sequence number, and exact in-memory
  payload; maximum TTL 300 seconds; old timers cannot clear a new lease;
- expiry/explicit clear perform sequence-number + exact Unicode payload check
  and `EmptyClipboard` within one `OpenClipboard` critical section, so an
  external copy cannot land between validation and clear;
- preserve all newer clipboard content and make explicit clear idempotent.

Timer closures capture only lease identity, not printable secret/error/path
metadata. Tests use a fake clipboard and deterministic scheduler; no real
developer clipboard access.

## 9. Mandatory deterministic tests

At minimum prove:

1. bundle version/schema, node/control split, source compatibility, control-only
   state, redacted formatting/reflection, and exact preservation on updates;
2. legacy plaintext migration at every write/read/move/delete crash point,
   conflict preservation, idempotent restart, and no plaintext fallback;
3. DPAPI flags/current-user scope, exact CreateFile parameters, complete
   read/write loops, zero-progress rejection, flush-close-move-readback order,
   1 MiB ciphertext cap, strict envelope, exact field comparison, and no send
   before verified read-back;
4. injected encrypt/decrypt partial allocation, `LocalFree`, open/size/read/
   write/flush/close/move/delete failures with exact resource counts; read-close
   failure blocks send; stale temp cleanup does not touch unrelated files;
5. every canonical-origin shared vector and malformed/userinfo/Unicode/
   encoded-host/IPv4/IPv6-zone/default-port case;
6. exact method/path/auth/body/status/schema for all seven routes and `/pair`;
   redirect/plaintext/timeout/cancel/oversize/depth/duplicate/trailing/unknown
   JSON and canary redaction;
7. recovery barrier/crash matrix, complete consume/probe table, no-secret
   restart probe, same-token 401 retry, 401->403 retention, explicit abandon;
8. concurrent service instances and both cancellation/promotion orderings:
   exactly one candidate crosses send per scope; other actor scopes progress;
9. recovery changes only control, promotion-before-delete restart convergence,
   and no duplicate request;
10. export is explicit and exactly three fields; clipboard history/cloud
    exclusion markers; copy contenders, copy-vs-clear both orders, old TTL vs
    new lease, external write at the former check/clear boundary, and newer
    clipboard survival;
11. secret/link/invite/token/URL/path canaries absent from errors, logs, normal
    formatting, filenames, config/plain files, and fake persistence outside
    the expected encrypted-value boundary;
12. all existing host tests and Windows amd64/arm64 compile/vet/test binaries.

Use barriers/hooks, not timing sleeps. Tests may only fail on timeout; timeout
must not establish correctness. No real network, DPAPI store, clipboard,
production credential file, or user data in host tests.

## 10. Verification and handoff

After the final edit run and record:

- focused migration/DPAPI/recovery/HTTP/clipboard suites at repetition;
- full uncached `go test ./...` and full `go test -race ./...`;
- `go vet ./...`, `go build ./...`;
- `GOOS=windows GOARCH=amd64` and `arm64`, `CGO_ENABLED=0`: vet, build, and
  test compilation for the root package;
- gofmt, `git diff --check`, scoped canary/log/plaintext scans;
- `task-board validate`;
- status and SHA-256 inventory for every changed/new file.

Create exactly one outcome named
`TASK-260712-47uve0_implementation-r1-results.md` with behavior, migration,
DPAPI fault, recovery response, clipboard, command/result, changed-file hash,
dirty-tree, and honest Windows runtime-gap matrices. Do not mark the task done;
set `to-review`. Native DPAPI/clipboard runtime, installed MSIX, and Windows
hardware evidence not run on this macOS host must remain explicit downstream
gates. Root line review, independent security/migration review, hash check, and
root reruns are mandatory.

## 11. Stop conditions

Stop and report rather than approximating if:

- exact UTS46 shared vectors cannot be satisfied by the reviewed dependency;
- a safe protected migration cannot distinguish source/destination/conflict;
- the Win32 abstraction cannot prove exact resource cleanup/send ordering;
- an HTTP response lacks required safe transition data;
- a real non-NULL clipboard owner/dispatcher cannot be exposed without UI
  ownership expansion;
- an allowed file has overlapping unexplained drift;
- any design would persist or silently expose plaintext recovery material.
