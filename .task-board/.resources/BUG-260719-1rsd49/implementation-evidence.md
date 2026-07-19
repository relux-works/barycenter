# Implementation evidence

- SQLite DSN applies busy_timeout(5000) and foreign_keys(ON) to every physical connection while ordered startup still applies busy_timeout, WAL, and foreign_keys.
- TestReplacementConnectionRetainsRequiredPragmas discards the sole physical connection with driver.ErrBadConn and verifies both pragmas on its replacement.
- Boolean-only external target readers are rejected fail-closed. Exact Store readers use persisted in-transaction ACL checks. Non-Store extensions must implement MediaTargetAuthorizationReader and keep the target decision valid through the descriptor-open callback.
- Production HTTP wiring checks that the exact Store is accepted. Generic HTTP fixtures now use persisted transmission targets; streamed-track fixtures use the lease contract.
- withControl is explicitly documented as preflight-only; existing stale-bearer and stale-role transaction-race coverage remains green.

Verification: go test ./internal/store ./internal/media ./cmd/duet-coordinator; go test ./...; go test -race ./... (internal/store 441.840s). All passed on 2026-07-19.