# Phase 1 independent security review — approval verdict

- **Decision:** APPROVE (accept)
- **Reviewer:** Claude Fable 5, task-board independent review session, run `RUN-260719-ca4eaf`, branch `review/task-260715-10ksxz-fable5`.
- **Reviewed revision:** exact `origin/main` head `1b9207e3ce57f6eb92955865da1e3f31d50f99f1` — a later exact main head than the AC-pinned `dab3999`, explicitly permitted by the acceptance criteria. The frozen audit (`docs/analysis/p1-independent-security-technical-audit.md`) is byte-identical at HEAD to the task-attached copy.
- **Date:** 2026-07-19.
- **Constraints honored:** read-only; no reviewed security path implemented; no production access. Working tree carries zero non-board changes.

## Non-implementation attestation

This session implemented none of the reviewed security paths. The three HIGH corrective fixes were authored by the autonomous pipeline and merged at `dab3999` (PR #74) before this review session existed; `git status` shows only `.task-board/**` progress files modified by this session. The engineering reviewer of record for the audit was `codex-inline-review`, a different identity. The independence requirement is satisfied.

## Revision handling — audit predates HEAD

The audit was frozen at `dab3999`; HEAD is `1b9207e`. The three security-critical source files changed between the two revisions (Phase 2/3 feature work), so each HIGH fix was re-verified for survival at HEAD rather than trusted from the audit:

- `cmd/duet-coordinator/main.go`: the P1-SEC-001 (`clientIP`, `rateLimiter.allow`, `rateLimitEntry`) and P1-SEC-002 (`newCoordinatorHTTPServer`) code is **unchanged** since `dab3999`; the +45 lines are an unrelated stream-accounting reconciliation loop.
- `cmd/duet-coordinator/onboarding.go`: the P1-SEC-001 forwarding helpers (`secureRequest`, `forwardedClientIP`, `directPeerIP`, `hasForwardingMarker`) are **unchanged** since `dab3999`.
- `internal/hub/hub.go`: the P1-SEC-002 admission path (256-slot `registerSlots` cap, 64 KiB read limit, 5 s deadline, HTTP-framing rejection) is intact and was **strengthened** post-audit — `awaitRegister` now also rejects any non-TEXT first frame.
- `coordinator/go.mod`, `pulsar-win/go.mod`, `acceptance/toolchains.json`, `ratelimit_test.go`: **not** in the `dab3999..HEAD` diff — the P1-SEC-003 pins are stable.

## HIGH findings — re-reviewed, remain closed

**P1-SEC-001 (pairing throttle / proxy spoof) — CLOSED, verified.** `clientIP` (main.go:96) grants forwarding authority only when `trustedProxy && r.TLS==nil && directPeerIP` is loopback && `secureRequest` passes; `secureRequest` (onboarding.go:525) requires exactly one `X-Forwarded-Proto: https` and one parseable final XFF hop (`forwardedClientIP`, onboarding.go:579). `X-Real-Ip` survives only as a presence marker in `hasForwardingMarker` (onboarding.go:553) and is never parsed as an IP authority (verified by exhaustive grep of non-test code). The rolling limiter reserves rejected attempts (`allow` appends before the `<= limit` test), caps per-key timestamps at `limit+1`, and bounds source keys at 4096 with oldest-key (`lastUsed`) eviction. Named tests pass at HEAD: `TestClientIPTrustedProxy`, `TestRateLimiterBoundsAttackerControlledKeysAndRejectedAttempts`.

**P1-SEC-002 (unbounded anonymous HTTP / WebSocket admission) — CLOSED, verified.** `newCoordinatorHTTPServer` (main.go:124) applies header/read/write/idle/header-size limits and is the wired transport (main.go:394). The hub admits at most 256 pending registrations, releases the permit on every handshake/auth outcome, keeps established sockets outside the pool, rejects query/body/HTTP-bearer framing (hub.go:357-359), and retains the 64 KiB first-frame limit + 5 s deadline. Named tests pass at HEAD: `TestCoordinatorHTTPServerHasBoundedPublicTransport`, `TestPendingRegistrationCapacityRejectsBeforeUpgradeAndRecovers`, `TestWebSocketRejectsCredentialBearingHTTPFraming` (incl. `http_bearer`).

**P1-SEC-003 (vulnerable Go toolchain) — CLOSED, verified.** `coordinator/go.mod` and `pulsar-win/go.mod` both pin `go 1.25.12`; `acceptance/toolchains.json` records `1.25.12`. `GOTOOLCHAIN=go1.25.12 go run golang.org/x/vuln/cmd/govulncheck@latest ./...` reports **No vulnerabilities found** for BOTH modules at HEAD.

No critical finding and no unresolved high finding remains.

## Medium dispositions — each explicitly accepted

- **M01 (WebSocket accepts browser `Origin`) — ACCEPTED.** `upgrader.CheckOrigin` returns true, but there is no cookie / ambient HTTP authority (no `Set-Cookie`/`http.Cookie` in the hub or coordinator handlers); the 256-bit bearer appears only in the bounded first protocol frame. Cross-origin traffic carries no tenant authority; the pending-registration cap + deadline + frame limit bound anonymous resource use.
- **M02 (legacy plaintext tailnet compatibility) — ACCEPTED.** All 45/45 self-service routes in `onboarding.register()` are wrapped by `api.secure` → `secureRequest`, which rejects non-loopback direct peers and requires TLS or the canonical loopback-proxy marker; `/pair` uses the forwarding-authority `clientIP`. A non-loopback proxy fails safe to a shared direct-peer limiter bucket. This is a compatibility boundary, not an Internet-deployment claim.
- **M03 (`/healthz` aggregate telemetry) — ACCEPTED, and post-audit expansion re-verified.** The public `/healthz` grew after the audit (`addStreamAccountingHealth`, `addLivePTTHealth`, `addPhase3Health`, `addMediaLifecycleHealth`). Every added surface was inspected and emits only aggregate booleans/counters/byte-totals/timestamps and one feature-state enum — no orbit/actor/slot identifiers, titles, credentials, media metadata, or audio. Detailed per-operator observability views are gated behind authenticated moderation-operator scopes, not the public endpoint.

## Migration MED-1 — routed finding, explicitly dispositioned NON-BLOCKING

**Finding (from the independent migration review, `TASK-260715-unbb7c`):** the P1-MIG-002 correction moved `busy_timeout`/`foreign_keys` off the DSN onto the pooled connection via `execStartupPragma`; `modernc/sqlite` can discard a file-backed connection when `sqlite3_is_interrupted` is observed at reset/put time, which a cancelled request context can trigger. A replacement connection created lazily by `database/sql` would run with `busy_timeout=0` and `foreign_keys=OFF` (WAL persists in-file) until restart.

**Independent assessment at HEAD:** the mechanism is reachable — `store.go` sets `SetMaxOpenConns(1)`, the DSN is only `?_txlock=immediate`, and request-scoped `ctx` reaches the driver via `AllowsMediaDownload` → `QueryRowContext` (transmission.go:1289, called from media/download.go:118). BUT this vector is an **ACL SELECT** (`mediaTargetACLQuery`), and neither `busy_timeout` nor `foreign_keys` changes SELECT results: FK enforcement does not alter read authorization, and a lost `busy_timeout` only converts lock contention into an immediate error (fail-closed), which with a single connection is already contention-free within the process. No secret, audio, tenant, or cross-orbit exposure results, and integrity failures are detectable at restart. The media-plane sub-review independently reached the same conclusion.

**Disposition:** ACCEPTED as non-blocking for this Phase 1 security signoff. It is a data-integrity/robustness hardening item, not a confidentiality issue, and does not weaken any authorization decision. Routed to a tracked engineering follow-up (below) so the finding does not disappear. Recommended fix: additionally carry `_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)` in the DSN as defense-in-depth for replacement connections, keeping `execStartupPragma` for first-connection WAL ordering.

## Trust-boundary matrix — all 11 boundaries verified at HEAD

Verified personally (legacy pairing/WS registration; rate limits/logs; Go toolchain; media owner ACL via MED-1) and via four independent read-only sub-reviews. Every boundary: PASS, with the audit's negative/adversarial protections enforced by real code (no stubs, no regressions) and backed by named tests that assert genuine denials:

- Bootstrap transport / bearer framing / capability separation — PASS. Duplicate/malformed Authorization rejected; node vs control digest-domain separation (exactly-one-match rule); revoked/left/disabled/stale binding denial re-resolved inside the writer transaction (TOCTOU-safe); invite/recovery single-winner + replay matrices.
- macOS Keychain + Windows DPAPI storage — PASS. `kSecUseDataProtectionKeychain` + after-first-unlock; user-scoped DPAPI (`LOCAL_MACHINE` deliberately unused); read-back-before-delete crash ordering; origin binding; one-time material single-use + send-barrier; full redaction with secret-canary tests.
- Legacy pairing + WebSocket registration — PASS (P1-SEC-001/002 above).
- Upload capabilities + untrusted media workers — PASS. Wrong-ID/token collapse to denial; fixed demuxers + protocol blacklist; `RLIMIT_CPU/AS/NOFILE/FSIZE` bound before exec; symlink/non-regular rejection; pre-worker container framing validation; stale-publication cancellation.
- Canonical media owner + immutable-target ACL — PASS. Cross-tenant owner denial; control vs node capability separation; ACL re-checked inside the transaction and held through `open(2)` with `SameFile`+size+digest; production wires the Store as the snapshot reader (strong persisted path).
- Explicit target / direct-ID resolution — PASS. Selector syntax only at HTTP; audience re-resolved live in the writer transaction; foreign/stale/expired references denied.
- DND + block precedence — PASS. Most-restrictive-wins with local as base (no orbit weakening, no emergency bypass); block precedes DND; ownership + revision/idempotency enforced; block cancels in-flight scheduling.
- Telegram identity + callbacks — PASS. Identity only from the authenticated Bot API update; private-chat single-consumption; opaque `tg1_`/HMAC-SHA256 keyed references with no embedded IDs; cross-user/query replay and too-late replacement denied; callback carries no authority (re-resolved at apply).
- History + action isolation — PASS. Credential-bound opaque cursors; 30-day window; target-detail redaction for non-owners; foreign-item denial; delete/replay/report/block re-authorized against current state.
- Moderation operator + evidence plane — PASS. Per-endpoint list/evidence/decide scopes with store-side recheck; hashed-only `mod_` credentials; accepted-target-bound + expiry + hash-verified evidence with audit-committed-before-bytes; trigger-enforced append-only moderation audit.
- Rate limits, logs, public policy — PASS. Bounded (4096) in-memory keys; rate-limit subjects stored only as SHA-256 digests (schema has no plaintext column); opaque fixed API errors; path/credential-free Telegram/worker errors; physical DB + log secret-absence tests; exact approved URLs/mailboxes/publication hashes.

Benign, by-design observations recorded (none blocking, none a leak): intra-orbit aggregate history visibility within one's own household (per-recipient identity redacted; cross-orbit denied); bounded plaintext moderation report *reason* text during the ≤30-day evidence window (scrubbed on retention; not a credential); `rate_limit_audit_events` insert-only in code without a DELETE/UPDATE trigger (append-only was only claimed for the moderation audit trail).

## Verification evidence (re-run at HEAD 1b9207e)

- `GOTOOLCHAIN=go1.25.12 go test ./...` (coordinator) — PASS.
- `GOTOOLCHAIN=go1.25.12 go test -race ./internal/store ./internal/hub ./cmd/duet-coordinator` — PASS (store 418s, hub, cmd all green).
- `GOTOOLCHAIN=go1.25.12 go test -race ./...` (pulsar-win) — PASS.
- `govulncheck` both modules — No vulnerabilities found.
- Named adversarial coverage tests — all PASS (listed per finding above).
- Swift gate is CI-authoritative (`swift test` is broken on this host by toolchain; the macos-15 `node-core` job runs it). Hosted CI run `29692957096` at HEAD `1b9207e` passed all four jobs: node-core, coordinator, pulsar-win, pulsar-win-packaged-probe.

## Non-blocking follow-ups created

A tracked follow-up captures MED-1 plus two defense-in-depth notes surfaced by the sub-reviews (all non-blocking, correct at HEAD): a future non-Store `MediaTargetSnapshotReader` must hold the target/block decision through the descriptor open; any future `withControl` handler must re-resolve lifecycle via the writer transaction rather than trusting `actor.Context`.

## Acceptance-criteria mapping

- Reviewer did not implement reviewed security paths — met (attested).
- Records name, revision (`dab3999` or later exact main head), findings, decision — met (`1b9207e`).
- P1-SEC-001..003 remain closed — met.
- Every critical/high finding fixed and re-reviewed — met (no criticals; three highs re-reviewed at HEAD).
- Each medium disposition explicitly accepted or converted to a blocking follow-up — met (M01/M02/M03 accepted; MED-1 accepted non-blocking + tracked follow-up).
- No secret/audio/tenant leak remains — met (verified across all boundaries, `/healthz` expansion, and public handlers).

On this approval, `TASK-260712-wy05n6` checklist item 1 is checked and that original task is accepted. This signoff makes no real-app or hardware claim; those remain in the manual epic.
