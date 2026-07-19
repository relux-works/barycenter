# Reviewer verdict: APPROVE

Terminal independent verdict for BUG-260719-1rsd49, issued by RUN-260719-0e576a per final-decision-brief.md, consolidating RUN-260719-f2757d, RUN-260719-c395cf, and this run.

## Head identity

- Reviewed production commit: da6b4cbe9307f9d60fb7c2192d5568a724f3fcbb (fix(coordinator): harden store authorization boundaries)
- Current HEAD: 5efb944580f6adfd2e938649b353b78924eb261f
- Verified this run: `git diff da6b4cb..HEAD --stat` touches only `.task-board/` files. Production code at HEAD is byte-identical to the reviewed commit. Working tree carries only board bookkeeping (this bug's progress.md).

## Boundary 1 — replacement SQLite connection pragmas (MED-1)

- store.go:105 DSN now carries `_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate`, so every lazily-created replacement connection gets both pragmas from DSN hooks without a process restart.
- Ordered startup sequence retained at store.go:114-119 (busy_timeout before WAL negotiation, then foreign_keys) via execStartupPragma — first-connection WAL ordering preserved.
- store_connection_test.go TestReplacementConnectionRetainsRequiredPragmas is deterministic: with SetMaxOpenConns(1) it discards the sole physical connection via conn.Raw returning driver.ErrBadConn, then asserts busy_timeout=5000 and foreign_keys=1 on the forced replacement — values can only come from the DSN, not the one-time startup Execs.

## Boundary 2 — media target reader contract

- download.go defines MediaTargetAuthorizationReader: any non-Store reader must hold the target decision through the descriptor-open callback (WithMediaDownloadAuthorization).
- SetTargetSnapshotReader now returns bool: exact backing Store (pointer-identity check against service.store) → persisted in-transaction path; lease-implementing reader → callback path; Boolean-only reader → targets and lease nil, fail-closed, returns false.
- Both OpenAuthorized and OpenAuthorizedStreamVariant wrap the store authorization + descriptor open inside the lease callback; !allowed maps to ErrMediaNotFound / ErrStreamTrackNotFound (fail-closed).
- Production wiring onboarding.go:402 rejects a false return with mediaDownloadInitErr "media download target store mismatch" — only the exact backing Store is accepted in production.
- download_test.go TestDownloadServiceRejectsExternalTargetSnapshotReader proves a Boolean-only reader is rejected with nil targets and the exact backing Store is accepted. cmd-level HTTP fixtures moved to the real contracts (lease-implementing reader with accepted-check; persisted transmission targets via CreateTransmission) — no harness avoidance.

## Boundary 3 — withControl preflight-only contract

- onboarding.go:692-697 documents withControl as routing/capability preflight only: Context is expected identity, never mutation authority; every handler must pass Bearer into a Store operation that re-resolves actor, orbit, role, and capability in its writer transaction.
- Named guard exists: onboarding_test.go:343 TestAuthenticatedMutationRechecksBearerAndRoleInsideTransaction with subtests stale_control_after_middleware (credential replaced after middleware → 401, replacement token works) and role_changed_after_middleware.

## Test evidence

- RUN-260719-f2757d (independent): reviewed all three boundaries, no HIGH issue; passed focused packages and fresh `go test -count=1 ./...`.
- RUN-260719-c395cf (independent): reviewed all three boundaries at identical HEAD, no HIGH issue.
- Producer at exact commit da6b4cb: `go test ./internal/store ./internal/media ./cmd/duet-coordinator`, `go test ./...`, and `go test -race ./...` all passed (internal/store 441.840s), 2026-07-19.
- This run, fresh at HEAD 5efb944 (short focused checks per brief): TestReplacementConnectionRetainsRequiredPragmas PASS (0.06s); TestDownloadServiceRejectsExternalTargetSnapshotReader PASS (0.07s); TestAuthenticatedMutationRechecksBearerAndRoleInsideTransaction PASS with both subtests (0.13s). No long or background test launched.

## Conclusion

No HIGH issue across three independent review passes. Implementation matches all acceptance criteria; the solution fits the project architecture (defense-in-depth DSN pragmas alongside retained startup ordering, fail-closed authorization boundaries, documented contracts with named regression guards); tests green with accumulated full-suite and race evidence at the exact reviewed commit. Verdict: APPROVE — BUG-260719-1rsd49 → done.
