# BUG-260719-1rsd49: store-connection-pragma-defense-in-depth

## Description
Non-blocking defense-in-depth hardening surfaced by the independent Phase 1 security review (TASK-260715-10ksxz, verdict APPROVE at 1b9207e) and the independent migration review (TASK-260715-unbb7c). None block Phase 1 acceptance; each is correct at HEAD and fails closed.

1) MED-1 SQLite pragma recycling: store.go uses SetMaxOpenConns(1) with pragmas applied only at open via execStartupPragma; the DSN carries only ?_txlock=immediate. modernc/sqlite can discard a file-backed connection on sqlite3_is_interrupted (cancelled request ctx reaches the driver via AllowsMediaDownload->QueryRowContext, transmission.go:1289). A lazily-created replacement connection would run busy_timeout=0 / foreign_keys=OFF until restart. Impact is integrity/robustness only — the reachable vector is an ACL SELECT whose result is unaffected by these pragmas; no secret/audio/tenant leak; fail-closed. Fix: additionally carry _pragma=busy_timeout(5000)&_pragma=foreign_keys(ON) in the DSN as defense-in-depth for replacement connections, keeping execStartupPragma for first-connection WAL ordering; or periodically assert pragma state.

2) Media reader contract: authorizeMediaDownload trusts an external MediaTargetSnapshotReader boolean when the reader is non-persisted (media_ingest.go). Production wires the Store (SetTargetSnapshotReader(st), onboarding.go:403 = strong persisted path with in-transaction ACL held through open), so this is test-only/unreachable today. Guard: any future non-Store reader must hold the target/block decision through the descriptor open, or the store must re-verify.

3) withControl gate is coarse (onboarding.go:669) and relies on the writer transaction (mutationActorContextTx) for authoritative disabled/left/satellite denial; all current withControl handlers re-resolve via the raw bearer. Guard: any future withControl handler must re-resolve lifecycle in the writer transaction rather than trust actor.Context.

## Scope
Coordinator Store connection initialization, generic media target authorization wiring, and control-middleware mutation authorization contract. No product behavior or external API expansion.

## Acceptance Criteria
Every initial and replacement SQLite connection has busy_timeout=5000 and foreign_keys=ON while startup still negotiates WAL; the exact backing Store uses persisted in-transaction target authorization, Boolean-only external readers fail closed, and any non-Store extension must hold an authorization lease through descriptor open; withControl documents its preflight-only guarantee and regression coverage proves stale control credentials cannot mutate after middleware; focused and full coordinator race tests pass.
