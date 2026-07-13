# TASK-260712-m5264f — independent R2 security review

Date: 2026-07-13
Role: reviewer, read-only
Branch / HEAD: main / e8bd240664a40b9cc78b974f3c34ad30712e2aa5
Verdict: PASS — no defect found in the reviewed onboarding/capability R2 scope.

## Reviewed authority and inventory

Read the complete task card, identity and onboarding-flow diagrams, original implementation guard, corrected R1 review, root R2 rework guard, independent R2 guard, full Rev15 contract and root amendments, accepted identity evidence, and specification sections 3.13, 6, 11, 12, 18, and 19. Reviewed the live combined worktree and every R2 production hunk plus shared identity.go and identity_schema.go. The worktree was already dirty and was not modified, reset, committed, pushed, or cleaned.

Exact reviewed SHA-256:

840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134  coordinator/internal/store/identity.go
6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f  coordinator/internal/store/identity_schema.go
77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9  coordinator/internal/store/onboarding.go
194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa  coordinator/internal/store/security_audit.go
8c2d5544a75cc09eb6b9b3980e91096a6ba8ef46e093c8fa12f847bc4f45cf2a  coordinator/internal/store/identity_migration_rework_test.go
08d8d49e269701a03bb4bbcf5be49f6e9fd71a54aa00a9fabc6f1fa96c566ec0  coordinator/internal/store/onboarding_rework_r2_test.go
d0c969f388d2b4138918c3e07490216c99c99f8565d4b90a39cb9238c53a1d1e  coordinator/cmd/duet-coordinator/onboarding.go
8b7e8582a7de081653e778e5d88fb6ba0db7858d5c813bdf3a40f4166ab7c350  coordinator/cmd/duet-coordinator/onboarding_rework_r2_test.go

Telegram boundary hashes remained exact:

583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go
a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go

Sibling Telegram consume remains an external boundary and is not accepted by this report.

## Security, protocol, and migration conclusions

1. Recovery rotation: PASS. identity_schema.go:121 defines a constrained normalized detail table linked to audit_events. security_audit.go:118 creates the typed base event and exact old/new detail together. onboarding.go:721 captures the prior handle before overwrite and onboarding.go:747 writes the detail inside the same immediate transaction. NULL first generation, collision retry, base/detail failure rollback, node/control preservation, and secret/digest scans have production-path tests. The repository is the sole production writer and always links details to recovery.rotated; direct arbitrary SQL is outside the application trust boundary.

2. Durable rate-limit audit: PASS. security_audit.go:16 defines stable typed classes; line 51 uses class-domain-separated SHA-256; lines 67-115 enforce full/nullable scope and real actor-membership coordinates while returning persistence errors. onboarding.go:409 reserves first, emits 429 only after the durable insert, and otherwise emits generic 500 without Retry-After. All seven HTTP N+1 paths are independently exercised. identity_schema.go:137 constrains event, class, digest, and class/scope shape. The deliberate absence of identity foreign keys is acceptable: production writes can occur only through repository validation, while historical positive orbit/actor coordinates survive later deletion. The schema does not admit an unsafe HTTP production path.

3. App-first alignment quarantine: PASS. identity_schema.go:637 rolls ordinary reconciliation back before quarantine; lines 716-766 re-read the violation, revoke and audit atomically in a separate immediate transaction, and propagate quarantine errors. The app-first shortcut verifies exact active-membership alignment at line 1026; the independent final serving gate repeats the invariant at line 1223. Real close/reopen fixtures cover cross-orbit and missing membership, no serving Store, durable disablement, credential/error redaction, quarantine audit failure, second failure before repair, explicit repair, and preserved app-first primary role.

4. Broader contract consistency: PASS. Existing production and negative tests cover exact endpoint envelopes, Cache-Control no-store, TLS and trusted-loopback proxy handling, duplicate authorization and bounded JSON, auth before syntax/reservation, node/control domain separation, lifecycle and role classification, constant-time/dummy-hash invalid paths, atomic invite/recovery races, hash-only persistence, legacy pair and websocket compatibility, feature-off behavior, migration and rollback coexistence, and audit atomicity. Node rejection before limiter reservation remains correct under Rev15 sections 7 and 10.

## Independent commands and results

- go test count=10 on focused R2 store tests and command HTTP R2 tests: PASS.
- go test race count=3 on R2 plus invite/recovery/concurrent limiter tests: PASS.
- relevant full identity/onboarding/recovery/invite/Telegram/rollback/migration/legacy/websocket tests: PASS.
- exact previoushead authority round trip, two-generation composition, and Telegram reconciliation: PASS in 8.071s.
- go test count=1 ./...: PASS; store 6.251s and every coordinator package green.
- go test -race count=1 ./...: PASS; store 31.396s and every coordinator package green.
- go vet ./...: PASS.
- go vet -tags previoushead ./internal/store: PASS.
- go build ./...: PASS.
- gofmt read-only check on all eight reviewed files: PASS.
- git diff --check plus untracked-file no-index whitespace checks: PASS.
- durable-sink and forbidden audit-field scans: PASS.
- Telegram boundary hash check: PASS.
- task-board validate before attachment: PASS.
- No R2 test skip was found.

## Findings by severity

Critical: none.
High: none.
Medium: none.
Low: none.

No external CI or distributed rate-limit durability is claimed. Local evidence exercised the production SQLite repository and HTTP handlers, real startup/reopen paths, independent database connections, and the exact pinned predecessor binary.