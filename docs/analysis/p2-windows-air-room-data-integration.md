# Phase 2 Windows Air data integration handoff

- Date: 2026-07-15
- Task: `TASK-260712-31zja2`
- Contract: `pulsar.air-lifecycle-policy.v1`

The packaged Windows app now exposes Air management as a first-class native
Win32 shell section. `AirAppClient` is the only Air wire adapter: it uses the
authenticated common routes, strict bounded same-origin JSON, opaque public
identifiers, per-mutation idempotency keys and canonical stable errors.
`WindowsAirComposition` owns polling, mutation serialization, stale-refresh
fencing and projection into the shell; the window procedure does not perform
network or lifecycle work.

## User-visible lifecycle

The Air screen shows saved memberships and exactly one current Air, plus the
local role, aggregate joined/active/online counts, both capacity ceilings,
effective invite/overlay/queue/replace policies and current playback routing.
The authorized API does not expose other-member identities, so the Windows UI
does not invent a directory or render opaque Air, membership or invite IDs.
Migrated two-party aliases appear by their human Air title.

Flows cover create, secure invite review, joining-primary save-only or
activate confirmation, decline, activate/switch/deactivate, leave, owner
dissolve, invite role/issue/withdraw and owner policy replacement. Switch,
deactivate, leave, dissolve and join-with-switch are armed first and execute
only after a distinct confirmation command; Escape/cancel clears the armed
action. Owner leave is not offered because the contract requires ownership
transfer or dissolve. Server revisions, role checks, capacity checks and the
active pointer remain authoritative.

The invite entry is a password-style edit control and is cleared immediately
after submission. An issued secret remains only in composition memory and is
never placed in window text, accessible names, formatting or durable state.
Explicit copy uses the existing hardened Windows clipboard backend, disables
clipboard history/cloud processing and compare-and-clears after 60 seconds
only when the value is unchanged. Hide, withdraw, expiry and shutdown zero the
in-memory projection.

## Accessibility, errors and dependency boundary

All interactive elements are native labelled buttons/edit controls in the
dialog tab order. `Ctrl+3` opens Airs, confirmation is textual rather than
color-only, and every control is laid out in DPI-scaled units under the
existing PerMonitorV2 shell. English and Russian copy covers unavailable
invites, both capacity ceilings, policy/role denial, revision and active-Air
conflicts, owner transfer, missing membership, unauthenticated credentials and
offline coordinator failures without treating failure as success.

This integration depends on `AirAppService`, the protected control credential
bundle and the existing shell projection only. It contains no Phase 1 draft
outbox, explicit-target, target-snapshot, inbox or public-discovery dependency;
those remain owned by later Phase 2 stories.

## Automated evidence

- `go test ./...`, `go vet ./...` and `go test -race ./...` pass in
  `pulsar-win`.
- `scripts/acceptance/run_automated.py --suite windows` passes all 7 stages:
  acceptance contract tests, tests/vet/race, Windows amd64 vet/build and
  Windows arm64 build.
- Blind `GOOS=windows GOARCH=amd64 go test -c .` passes for the packaged native
  shell source.
- Unit coverage pins all common lifecycle routes, same-origin authentication,
  strict/error responses, secret redaction/expiry, stale-refresh fencing,
  explicit disruptive confirmation, EN/RU copy, keyboard/native-control seams
  and the Phase 1/target/inbox dependency boundary.

No real-app, screen-reader, physical-hardware or live high-DPI result is
claimed here; those remain in the separate manual testing epic.
