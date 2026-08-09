## Status
backlog

## Review
required

## Task Class
code

## Estimate
estimated(fibonacci(5))

## Blocked By
- (none)

## Blocks
- (none)

## Checklist
- [ ] Reproduce three same-identity GUI instances and correlated WebSocket close-1006 storm
- [ ] Add per-user single-instance ownership and existing-window activation
- [ ] Recover safely from stale lock and crashed process ownership
- [ ] Add regression tests for duplicate launches and stable authenticated session
- [ ] Verify Air Join commits with one instance and helper binaries remain unaffected

## Notes
2026-07-28 manual mitigation verified: after terminating the three contending instances and owner launching exactly one installed binary, process count remained 1 with stable PID and two established 443 connections across 30s; coordinator remained nodes_connected=2. Product bug remains open because per-user single-instance enforcement is not implemented.

## Precondition Resources
(none)

## Outcome Resources
(none)

## Created
2026-07-27T22:58:50Z

## Last Update
2026-07-27T23:12:51Z
