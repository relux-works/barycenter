## Status
done

## Assigned To
[reviewer] reviewer (codex)

## Created
2026-07-12T15:30:16Z

## Last Update
2026-07-13T14:46:11Z

## Blocked By
- TASK-260712-3v1k7q
- TASK-260712-1bpog0

## Blocks
- TASK-260712-2u1w16
- TASK-260712-47uve0
- TASK-260712-38qsku
- TASK-260712-2qpp6w
- TASK-260712-1bnos4
- TASK-260712-2vhf80

## Checklist
- [x] Implement endpoint contracts and error codes
- [x] Issue separate node and control credentials with one-time recovery material
- [x] Add capability middleware and negative auth coverage
- [x] Preserve legacy pair behavior and feature-flag gating
- [x] Use constant-time hash checks, atomic single-use consume and plaintext-secret redaction
- [x] Return recovery material once and persist only its hash
- [x] Code written per task description and AC
- [x] Relevant tests written for new or changed behavior and passing
- [x] Lint clean
- [x] Relevant build/validation commands run after changes and build not broken
- [x] New outcome artifact attached on the board with a task-scoped name when the work produces notes, logs, screenshots, or other deliverables
- [x] Important findings, decisions, anomalies, or regressions recorded in logbook when relevant
- [x] Implementation matches AC
- [x] Solution fits project architecture
- [x] Tests green
- [x] If problems found — notes added and status set to to-dev

## Notes
spawn queued: [implementer] developer (codex) (run=RUN-260713-9b24a6, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-9b24a6)
Implementation and focused/full uncached tests are passing. First full race rerun encountered a transient sibling-owned compile boundary in cmd/duet-coordinator/telegram_identity_test.go (errors.Is added before errors import); preserving sibling ownership and will rerun after the shared file settles.
Developer implementation and local verification are ready for review. Outcome: TASK-260712-m5264f_results.md (SHA-256 9066b4d0767164bd0ba20cee79ff5dcf9ba48c6fe0e691a155d21dc8fb755746). Full uncached coordinator suite, full race, focused critical race, identity/rollback matrix, vet, build, gofmt, and diff checks pass. Fresh security/protocol/migration plus root hash audit required because identity.go/identity_schema.go accepted hashes changed. External boundary: sibling TASK-260712-2xkyot owns Telegram consume; its post-terminal T10 deterministic pre-BEGIN race follow-up remains for sibling review/rework and was not edited across ownership.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-9b24a6, pid=62296, exit=0)
spawn queued: [reviewer] reviewer (codex) (run=RUN-260713-2d577b, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260713-2d577b)
Independent reviewer verdict: back to development. Two frozen-contract audit blockers remain despite all focused/full/race/vet/build/format/diff/board checks passing. (1) Recovery rotation overwrites recovery_id and records only recovery.rotated; Rev15 section 7 requires the audit to retain both old and new non-secret recovery_id handles. The current audit_events schema cannot represent them, so this is a release-blocking contract/schema inconsistency unless resolved by an additive compatible audit detail mechanism or an explicit accepted contract erratum. (2) HTTP 429 paths call the best-effort generic LogEvent logger, whose insert errors are ignored, rather than durably recording all 429 events in the security audit trail as Rev15 section 12 requires. Add durable/error-checked audit representation and production-path coverage for rotation details, rollback, each limiter class, and audit-write failures. Full evidence: TASK-260712-m5264f_security-review-r1.md (SHA-256 4b41ec4f40aeb855f356f4931fe674d2398fdb44379f37494b8a77ae1048cd5e). No production, test, checklist, logbook, commit, or push changes were made.
Independent review amendment after root audit directives: the attached review now records four blockers and supersedes the earlier two-finding summary. Added F3: identity_schema.go:879-885 skips ensureMembershipTx for paired_by=0 app-first credentials with control material, so a credential bound to orbit A plus an active membership in orbit B passes current reconciliation, serving-gate, and foreign-key checks; endpoint joins reject it later, but Rev15 17.5 requires fatal-disable/refuse-serving. Add an explicit alignment assertion and executable reopen/serving-gate fixture. Added F4: withControl rejects valid node principals before rotate/link body validation and per-actor reservation; Rev15 9 requires authenticated syntactically-valid wrong-capability attempts to count. Require malformed-node 400/no-count, ten valid-node 403/count, and eleventh exact 429 tests. Device-invite issue has no frozen per-actor limiter and is not part of F4. Revised outcome: TASK-260712-m5264f_security-review-r1.md (SHA-256 bb2def3c8c0fde2c385012b2e783e9139490c68685297c811390ec60d318630c). Verdict remains back to development; no code or shared-file changes.
Reviewer evidence correction (supersedes both prior review-resource SHA notes): direct shasum proved the earlier review draft contained incorrectly transcribed file hashes. The task-owned exact SHA-256 inventory is now corrected in the attached outcome: main ebac82641471039ec0dcb66e3f4fa8f49b543d38a71aa5caa9e56f030e26039d; cmd onboarding 6aa01fd1ee8f34526ebfba9db4807e468c46850b0e13bcb38ba6510a2a3064c3; cmd tests 02d18a2c64dc3d39f447d5057ca0a5cfa735f94cc7f1da82d9fdcb359213fd95 and f37935809bd369df543b9e2a67333a81a6c8da3f73e8fa6da081f559cb08e0b4; identity dcd4cc3c1188569439335c1742c657cb4235aec223f1d2ed5f4cb4fcde0de5dd; identity schema 892238d4d8d6aa3adbeb7c9a1009df693d84fb9803c5fa21b718521ca33472bb; store onboarding 66948069ac47d7a8f2f21718149f129a1ff89bba8b47b20fe863a550e5c1ea43; store tests 4e059c4684cc9fb78e19a12217ed5c090ad64f9dfb8a9fce595daef1898612a7; both producer outcome copies 9066b4d0767164bd0ba20cee79ff5dcf9ba48c6fe0e691a155d21dc8fb755746. Corrected review outcome TASK-260712-m5264f_security-review-r1.md SHA-256: a77ca1d3b565fafa11feb5f02478c391a12e13314c6277e86e8c67a58fdbc1cf. It explicitly resolves root directives as F3 (HIGH app-first orbit-alignment serving-gate bypass) and F4 (MEDIUM node rotate/link rate-order violation), in addition to F1-F2. Verdict remains back to development.
Final reviewer correction (supersedes all earlier review summaries/hashes): the node rotate/link limiter question is NOT A DEFECT. Rev15 7 step 1 and 10 step 1 specifically require node tokens to fail 403 in auth middleware before step 2 syntax and step 3 reservation; section 9 counts lifecycle/role 403s, not the wrong credential-capability class. Current node zero-count behavior is therefore correct, while satellite/left/disabled control attempts count. Device-invite issue likewise has no frozen per-actor limiter. Final blockers are exactly three: F1 rotation audit lacks old/new recovery_id details; F2 all 429s go only to best-effort generic events rather than durable security audit; F3 app-first paired_by=0/control reconciliation skips the only cross-orbit membership check and can pass the serving gate. Final corrected outcome: TASK-260712-m5264f_security-review-r1.md, SHA-256 a3d0c001332c485de965e6a6a1e1bcd1cbe45c806865bdd5736b303a8d5f6559. Direct scans confirm the discarded erroneous hash prefixes and four-blocker/F4-finding text are absent. Verdict: back to development.
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-2d577b, pid=658, exit=0)
spawn queued: [implementer] developer (codex) (run=RUN-260713-b73830, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-b73830)
R2 developer rework resolves the three root blockers: exact transactional recovery transition detail, durable typed audit-before-429 for seven HTTP limiter classes with strict scope validation, and fatal app-first orbit-alignment quarantine plus independent serving gate. Focused repetitions, compatibility, exact previous-head, full uncached, full race, vet including previoushead, build, formatting, whitespace, secret-field, and untouched Telegram hash checks pass. Outcome TASK-260712-m5264f_rework-r2-results.md SHA-256 e41c49d02d8ce0ed6e6278a14cdd1d2a3ae7d5c895a781cbd6dc9110bf1a613b. Fresh independent and root review remain required.
R2 evidence refresh after root F2b directive: direct SQLite checks now reject all three invalid persistence shapes (half scope, scoped pre-identity, unscoped actor), valid rows assert security.rate_limited plus nonzero sane timestamps, and unchanged row counts are proven. Focused R2 passed 10/10; exact previous-head, full uncached, full race, vet, previoushead vet, build, formatting, security scans, and Telegram boundary hashes were rerun after the edit. Updated outcome SHA-256 41acf63c05e3adb73849860791ead3a13b14f500abf521c962fd7321e3ef61a8.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-b73830, pid=10319, exit=0)
spawn queued: [reviewer] reviewer (codex) (run=RUN-260713-5ae8e5, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260713-5ae8e5)
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-5ae8e5, pid=25546, exit=0)
ROOT ACCEPTANCE R2: accepted only after root restored the reviewer auto-done transition to to-review, audited production and tests line by line, reproduced focused/full/race/previous-head/vet/build/static gates, froze exact hashes, and read the clean independent R2 report. Telegram consume remains outside this acceptance and proceeds under its separate R4 durable-audit guard. Evidence: TASK-260712-m5264f-root-acceptance-r2.md.

## Precondition Resources
- [p1-identity-model.puml](file://TASK-260712-m5264f/p1-identity-model.puml) — Identity model used by the onboarding and capability APIs
- [p1-onboarding-flows.puml](file://TASK-260712-m5264f/p1-onboarding-flows.puml) — Create, join, recover, and Telegram link flow sequence
- [TASK-260712-m5264f-implementation-guard.md](file://TASK-260712-m5264f/TASK-260712-m5264f-implementation-guard.md) — Mandatory onboarding/capability implementation and evidence guard
- [TASK-260712-m5264f-independent-review-r1.md](file://TASK-260712-m5264f/TASK-260712-m5264f-independent-review-r1.md) — Read-only independent security, protocol, migration and contract-consistency review required before root acceptance
- [TASK-260712-m5264f-rework-guard-r2.md](file://TASK-260712-m5264f/TASK-260712-m5264f-rework-guard-r2.md) — Root mandatory R2 guard for durable audit, rotation detail, and app-first alignment quarantine
- [TASK-260712-m5264f-independent-review-r2.md](file://TASK-260712-m5264f/TASK-260712-m5264f-independent-review-r2.md) — Mandatory read-only independent R2 security, protocol, migration, hash, and test review before root acceptance

## Outcome Resources
- [TASK-260712-m5264f_spawn-log_-implementer--developer--codex-.log](file://TASK-260712-m5264f/TASK-260712-m5264f_spawn-log_-implementer--developer--codex-.log) — System spawn log captured by task-board
- [TASK-260712-m5264f_results.md](file://TASK-260712-m5264f/TASK-260712-m5264f_results.md) — Code, security invariants, endpoint/AC/test mapping, hashes, command evidence, dirty-tree scope, and review boundaries
- [TASK-260712-m5264f_spawn-log_-reviewer--reviewer--codex-.log](file://TASK-260712-m5264f/TASK-260712-m5264f_spawn-log_-reviewer--reviewer--codex-.log) — System spawn log captured by task-board
- [TASK-260712-m5264f_security-review-r1.md](file://TASK-260712-m5264f/TASK-260712-m5264f_security-review-r1.md) — Final corrected independent security/protocol/migration review; three blockers; node limiter challenge resolved; verdict back to development
- [TASK-260712-m5264f_rework-r2-results.md](file://TASK-260712-m5264f/TASK-260712-m5264f_rework-r2-results.md) — R2 implementation, exact hashes, AC mapping, failure injection, compatibility, race, and validation evidence
- [TASK-260712-m5264f_security-review-r2.md](file://TASK-260712-m5264f/TASK-260712-m5264f_security-review-r2.md) — Independent read-only R2 security, protocol, migration, hash, and test review
- [TASK-260712-m5264f-root-acceptance-r2.md](file://TASK-260712-m5264f/TASK-260712-m5264f-root-acceptance-r2.md) — Root line-by-line, hash, security, migration, race, and test acceptance after R2 and independent review
