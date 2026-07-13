## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:30:17Z

## Last Update
2026-07-13T20:38:07Z

## Blocked By
- TASK-260712-3v1k7q
- TASK-260712-m5264f

## Blocks
- TASK-260712-38qsku
- TASK-260712-3dqc3l
- TASK-260712-1x9ruo

## Checklist
- [x] Define the credential bundle and migration path
- [x] Add API client calls for create, join, recover, and Telegram link
- [x] Keep existing pairing state intact during upgrade
- [x] Cover Keychain migration and split-credential tests
- [x] Prove recovery secret is not silently persisted and pasteboard or logs are cleaned
- [x] Redact invite and Telegram deep links while preserving Keychain migration
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
Root implementation guard R1 attached; SHA-256 4360eb783b4d6c8b378d4497e926aa598baec538ad57ad0916dff14d4a7225d8. Producer must leave output for root audit and independent security/migration review.
spawn queued: [implementer] developer (codex) (run=RUN-260713-2fd8d9, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-2fd8d9)
Implementation handoff evidence attached as TASK-260712-2u1w16_results.md. Root findings M1-M12 are addressed and mapped to deterministic tests. Post-audit hardening reconciles mixed ever_sent DP/login pending copies before any restart probe; partial transition failure produces zero sends. Focused 53/53 and full 103/103 Swift tests pass; recovery/pasteboard suites pass 20 repeated runs; release NodeApp build, strict scoped formatting, diff check, API and secret scans pass. Tests used only injected stores/transports/pasteboards and did not touch real Keychain or user data. Release build retains pre-existing out-of-scope PlayerCore Sendable warnings. Narrow main.swift scope expansion is limited to nonzero failure on protected pairing-save error.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-2fd8d9, pid=32002, exit=0)
spawn queued: [reviewer] reviewer (codex) (run=RUN-260713-b65cad, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260713-b65cad)
Independent R2 verdict: back to development. HIGH R2-F1: ever_sent=false matching-active recovery is accepted and pending is deleted without authenticated proof. HIGH R2-F2: permissive destination decoding treats non-equivalent DP/login protected payloads as equal and deletes one copy. MEDIUM R2-F3: limited-context classification is lost after promotion succeeds but pending deletion fails. MEDIUM R2-F4: UTS46-mapped trailing root dot is rejected due canonicalization order. Actual production-source probes reproduce all four. Focused 53, full 103, 50 repeated recovery/export runs, formatting, diff check, privacy scans, and release build pass but omit these adversarial schedules. Required corrections and exact hashes/commands are in TASK-260712-2u1w16_independent-review-r2.md. No real Keychain or pasteboard was touched.
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-b65cad, pid=67536, exit=0)
Root R3 rework guard attached; SHA-256 142f148b9cf7ef177c1ff4f9710ef2e8a955234b36fe04eca7b7648e1a55af7f. Four independent R2 findings plus checked-close export, value-independent descriptions, and retry-safe clipboard lease are mandatory before fresh review.
spawn queued: [implementer] developer (codex) (run=RUN-260713-7d0eff, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-7d0eff)
R3 rework evidence attached as TASK-260712-2u1w16_rework-r3-results.md (SHA-256 293ba3fc4d74ea4521fdbc7d2822b5121eb5c25cf918255932885201be11e32c). R3-F1 through F7 are mapped to production seams and deterministic tests. Focused onboarding 67/67, full Swift 117/117, isolated recovery/clipboard repetitions 100/100, release build, strict formatting, diff check, privacy scans, and board validation pass. Release build has only pre-existing out-of-scope Sendable warnings. Tests used injected fakes and did not touch real Keychain, pasteboard, user files, or live network. Root audit and fresh independent review remain mandatory.
agent completed: [implementer] developer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-7d0eff, pid=75553, exit=0)
spawn queued: [reviewer] reviewer (codex) (run=RUN-260713-f9f020, max_parallel=20)
spawn run started: [reviewer] reviewer (codex) (run=RUN-260713-f9f020)
R4 independent review verdict: BACK TO DEVELOPMENT. HIGH: RecoveryPasteboardLease silently exhausts its capped automatic clear retries while retaining the secret-bearing lease/pasteboard payload, with no terminal failure visibility for UI/manual remediation (RecoveryExport.swift:257-260,280-289). MEDIUM: non-429 error envelopes accept malformed non-null retry_after_seconds values because invalid positiveInt conversion collapses to nil (OnboardingHTTPClient.swift:584-600); isolated production-seam test reproduced acceptance for string, boolean, object, array, zero, negative, and fractional values. Existing focused 67 tests, full 117 tests, 100x recovery/clipboard repetition, strict formatting, diff check, board validation, canary scans, and release build pass. Required corrections/tests and frozen hashes are in outcome TASK-260712-2u1w16_independent-review-r4.md (SHA-256 3b3127cb16215391d172de15b446ecf592b802d323b0810ca205fac6bf9b12d7).
agent completed: [reviewer] reviewer (codex) (exit=0)
spawn run completed: codex (run=RUN-260713-f9f020, pid=89642, exit=0)
Root R5 rework guard attached; SHA-256 721a52cf0a2754d4a63370169284e3e43c9873d0017ea9ef3abf7d308d9b1553. Mandatory: strict origin-bound backup acknowledgement, exact create-title echo, observable retryable terminal clipboard cleanup failure, and strict retry_after_seconds/header parsing. Root audit and fresh independent review remain required.
spawn queued: [implementer] developer (codex) (run=RUN-260713-75a26c, max_parallel=20)
spawn run started: [implementer] developer (codex) (run=RUN-260713-75a26c)
2026-07-14: strict sequential inline execution started outside task-board spawn workflow on branch task/task-260712-2u1w16-macos-keychain-r5. Markdown and board tracking enabled; next task remains untouched until acceptance and landing.
R5 handoff attached as TASK-260712-2u1w16_rework-r5-results.md (SHA-256 e862d68b9aacdfaac3fbb0c3472245feb2b3e858212d22fb9cb6b7b5eab81199). R5-F1..F4 are present in commit 99aa26c and audited in full. Focused 75/75, full 125/125, recovery/clipboard stress 100/100 at 31 tests per run, release build, strict six-file formatting, diff check, privacy/URL scans, and board validation pass. No real user state was touched. Per the user-mandated inline mode, the same executor performed a separate cold falsification pass; independent reviewer identity is not claimed.
ROOT INLINE ACCEPTANCE 2026-07-14: complete-file audit and adversarial cold pass found no new issue; all technical AC and R5-F1..F4 evidence gates pass. User-required self-execution mode supersedes reviewer-identity separation only; no independent-review claim is made. Task accepted at frozen hashes recorded in TASK-260712-2u1w16_rework-r5-results.md.

## Precondition Resources
- [p1-onboarding-flows.puml](file://TASK-260712-2u1w16/p1-onboarding-flows.puml) — macOS onboarding client flow sequence
- [TASK-260712-2u1w16-implementation-guard-r1.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16-implementation-guard-r1.md) — Root-reviewed macOS credential, migration, HTTP, recovery, privacy, and verification contract R1
- [TASK-260712-2u1w16-independent-review-guard-r2.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16-independent-review-guard-r2.md) — Root independent security and migration review guard R2
- [TASK-260712-2u1w16-rework-guard-r3.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16-rework-guard-r3.md) — Root R3 rework contract for four independently reproduced and three root-audit findings
- [TASK-260712-2u1w16-independent-review-guard-r4.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16-independent-review-guard-r4.md) — Root independent security and migration review guard R4 over frozen R3 bytes
- [TASK-260712-2u1w16-rework-guard-r5.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16-rework-guard-r5.md) — Root R5 rework contract for two independent R4 findings and two root-reproduced binding defects

## Outcome Resources
- [TASK-260712-2u1w16_spawn-log_-implementer--developer--codex-.log](file://TASK-260712-2u1w16/TASK-260712-2u1w16_spawn-log_-implementer--developer--codex-.log) — System spawn log captured by task-board
- [TASK-260712-2u1w16_results.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16_results.md) — Implementation behavior matrices, M1-M12 evidence, verification results, hashes, and remaining risks
- [TASK-260712-2u1w16_spawn-log_-reviewer--reviewer--codex-.log](file://TASK-260712-2u1w16/TASK-260712-2u1w16_spawn-log_-reviewer--reviewer--codex-.log) — System spawn log captured by task-board
- [TASK-260712-2u1w16_independent-review-r2.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16_independent-review-r2.md) — Independent R2 security and migration review with reproduced findings, hashes, and verification evidence
- [TASK-260712-2u1w16_rework-r3-results.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16_rework-r3-results.md) — R3-F1..F7 implementation map, migration/recovery matrices, verification results, dirty-tree boundary, and SHA-256 inventory (artifact SHA-256 293ba3fc4d74ea4521fdbc7d2822b5121eb5c25cf918255932885201be11e32c)
- [TASK-260712-2u1w16_independent-review-r4.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16_independent-review-r4.md) — Independent R4 security and migration review: BACK TO DEVELOPMENT; strict error-envelope and clipboard terminal-liveness findings
- [TASK-260712-2u1w16_rework-r5-results.md](file://TASK-260712-2u1w16/TASK-260712-2u1w16_rework-r5-results.md) — R5-F1..F4 implementation map, exact hashes, 75/125 test evidence, 100x stress, release/format/privacy gates, and inline cold-review disclosure
