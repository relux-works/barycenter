# Phase 1 security and privacy technical audit

Date: 2026-07-15
Task: `TASK-260712-wy05n6`
Engineering reviewer: `codex-inline-review`
Independent approver: Ivan Oparin (separate owner gate)

## Result

The engineering audit found three high-severity release blockers. All three
were fixed in this change and received deterministic negative coverage. No
critical finding and no unresolved high finding remains in the reviewed Phase
1 source. This document is a technical input, not the required independent
sign-off: the author also implemented the corrections below.

## Findings

| ID | Severity | Exploit path and impact | Disposition |
|---|---|---|---|
| P1-SEC-001 | High | With `trusted_proxy` enabled, legacy `/pair` accepted `X-Real-Ip` or `X-Forwarded-For` from an arbitrary remote peer. An attacker could rotate forged source keys to bypass the pairing-code throttle. The limiter also retained an unbounded number of still-live attacker keys. This enabled online code guessing, database load and process-memory exhaustion. | Fixed. Forwarding authority now requires a plaintext loopback peer, the canonical `X-Forwarded-Proto: https` marker and exactly one parseable XFF field. Direct TLS, remote peers, duplicate/malformed fields and X-Real-Ip fall back to the direct peer. The rolling limiter reserves rejected attempts and caps both timestamps and source keys. Covered by `TestClientIPTrustedProxy` and `TestRateLimiterBoundsAttackerControlledKeysAndRejectedAttempts`. |
| P1-SEC-002 | High | The public `http.ListenAndServe` used no header/read/write/idle limits, and `/ws` admitted unlimited unauthenticated upgraded sockets for the five-second registration window. Slow HTTP bodies/headers and registration floods could exhaust descriptors, goroutines and memory before bearer authorization or application rate limits ran. | Fixed. The coordinator uses a bounded `http.Server` with header, request, response, idle and header-size limits. The hub admits at most 256 pending registrations, releases the permit on every handshake/auth outcome, preserves established sockets outside that pool, rejects query/body/HTTP-bearer framing, and retains its 64 KiB first-frame limit plus five-second deadline. Covered by `TestCoordinatorHTTPServerHasBoundedPublicTransport`, `TestPendingRegistrationCapacityRejectsBeforeUpgradeAndRecovers`, and `TestWebSocketRejectsCredentialBearingHTTPFraming`. |
| P1-SEC-003 | High | `coordinator/go.mod` and `pulsar-win/go.mod` pinned `go 1.25.0`, so CI and release jobs selected a standard library predating all subsequent security patches. A symbol scan under the locally installed Go 1.26.0 reached 15 affected standard-library advisories, including TLS privacy/authentication and resource-exhaustion paths. | Fixed. Both release modules require Go 1.25.12, the July 2026 security release. `GOTOOLCHAIN=go1.25.12 go run golang.org/x/vuln/cmd/govulncheck@latest ./...` reports `No vulnerabilities found` for both modules. Primary references: [Go release history](https://go.dev/doc/devel/release#go1.25.12) and [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856). |

## Explicit medium dispositions

| ID | Concern | Disposition |
|---|---|---|
| P1-SEC-M01 | The WebSocket upgrader accepts browser `Origin` values. | Accepted for Phase 1. Pulsar is native on both platforms and native stacks may attach an Origin. There is no cookie or other ambient HTTP authority: the 256-bit bearer appears only in the bounded first protocol frame. Cross-origin traffic therefore has no tenant authority, while the new pending-registration cap, deadline and frame limit bound anonymous resource use. Revisit if a browser client or cookie authentication is introduced. |
| P1-SEC-M02 | Legacy `/pair`, `/media/*` and `/ws` retain direct plaintext tailnet compatibility. | Accepted compatibility boundary, not an Internet deployment claim. Production must keep the coordinator port behind the approved TLS terminator/firewall; application self-service routes independently reject non-TLS remote peers. A non-loopback proxy now fails safe to a shared direct-peer limiter bucket rather than trusting spoofable headers. |
| P1-SEC-M03 | Public `/healthz` exposes aggregate orbit and connected-node counts. | Accepted operational telemetry. It contains no orbit/actor/slot identifiers, titles, credentials, media metadata or audio. Per-tenant presence remains authenticated and closed to capture/device/process detail. |

## Trust-boundary matrix

| Boundary | Source inspected | Negative/adversarial evidence |
|---|---|---|
| Bootstrap transport, bearer framing and capability separation | `cmd/duet-coordinator/onboarding.go`, `internal/store/identity.go`, `internal/store/onboarding.go` | malformed/duplicate authorization; remote plaintext and forwarding spoof rejection; node/control digest-domain collision; revoked, left, disabled and stale binding denial; invite/recovery single-winner and replay matrices |
| macOS Keychain and Windows DPAPI storage | `node-app/Sources/NodeCore/Keychain.swift`, `RecoveryService.swift`, `pulsar-win/protected_repository.go`, `recovery_service.go` | protected read-back, migration crash points, origin binding, recovery pending-generation races, redacted descriptions and one-time material tests |
| Legacy pairing and WebSocket registration | `cmd/duet-coordinator/main.go`, `internal/hub/hub.go` | forged forwarding fields, bounded source state, invalid protocol/capabilities/token, oversized first frame, pending-registration saturation/recovery and credential-bearing HTTP framing |
| Upload capabilities and untrusted media workers | `media_upload.go`, `internal/store/media_ingest.go`, `internal/media/submit.go`, `media.go`, `runner_linux.go` | wrong ID/token collapse, expired/finalized sessions, offset/finalize races, actual-byte limits, fixed demuxers, disabled protocols, worker queue/CPU/memory/file/output limits, symlink/non-regular inputs and stale publication cancellation |
| Canonical media owner and immutable-target ACL | `internal/media/download.go`, `internal/store/media_ingest.go`, `internal/store/transmission.go` | cross-tenant owner denial; control/node capability separation; exact actor/slot/binding generation; block/delete/revoke races held through descriptor open; storage identity and digest checks |
| Explicit target/direct-ID resolution | `transmission_http.go`, `internal/store/transmission_resolution.go` | selector syntax only at HTTP; active domain, role, membership, slot and credential generation re-resolved in the writer transaction; foreign and stale direct IDs denied |
| DND and block precedence | `presence_policy_http.go`, `internal/store/transmission_policy.go`, `presence_policy_surface.go` | local-over-orbit precedence, no emergency bypass, actor/orbit ownership checks, revision/idempotency conflicts, active block revocation of scheduling/download/report access |
| Telegram identity and callbacks | `internal/bot/bot.go`, `callback.go`, `internal/store/telegram_inline_routing.go`, `telegram_history_callbacks.go` | verified Bot API identity, private-chat link consumption, opaque HMAC references, chat/message/actor/role/orbit binding, expiry, query replay, cross-user replay and too-late replacement denial |
| History and action isolation | `history_http.go`, `internal/store/history_query.go`, `internal/historyactions/service.go` | actor/credential-bound cursors, 30-day window, target-detail redaction, foreign item denial, delete/replay/report/block authorization rechecked against current state |
| Moderation operator and evidence plane | `moderation_http.go`, `internal/store/moderation.go`, `internal/moderation/service.go`, `internal/media/download.go` | separate `mod_` credentials and least-privilege list/evidence/decide scopes, revocation, report ownership, accepted-target proof, evidence expiry/hash/descriptor validation and append-only evidence/action audits |
| Rate limits, logs and public policy | `main.go`, `onboarding.go`, `security_audit.go`, Telegram safe-error boundary, policy/site validators | bounded in-memory keys, durable subject digests without plaintext, fixed API errors, credential/path-free worker and Telegram errors, secret absence in DB/log tests, exact approved URLs/mailboxes and publication hashes |

## Verification evidence

The working tree passed these gates on 2026-07-15 before the task commit:

```text
GOTOOLCHAIN=go1.25.12 go test ./...                         # coordinator
GOTOOLCHAIN=go1.25.12 go test -race ./internal/store ./internal/hub ./cmd/duet-coordinator
GOTOOLCHAIN=go1.25.12 go run golang.org/x/vuln/cmd/govulncheck@latest ./...
GOTOOLCHAIN=go1.25.12 go test -race ./...                   # pulsar-win
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test
```

Both exact Go 1.25.12 vulnerability scans reported `No vulnerabilities
found`. The coordinator full and focused race suites, the complete Windows
race suite, and all 218 macOS tests in 35 suites passed. After committing, the
same commit must also pass the clean-worktree automated acceptance suite and
all hosted jobs before merge.

Hosted CI remains authoritative for Linux worker limits, Windows cross-build
and packaged-probe contracts. Real-device and real-application observations
remain in manual epic `EPIC-260714-th54l3`; this audit makes no hardware claim.

## Independent approval request

Ivan Oparin must independently confirm that the report covers every listed
trust boundary, reproduce or inspect the negative evidence, challenge the
three fixes, and either accept each medium disposition or create a blocking
follow-up. Until then checklist item 1 and the original task acceptance remain
open.
