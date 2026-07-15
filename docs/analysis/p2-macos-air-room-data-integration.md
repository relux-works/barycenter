# Phase 2 macOS Air data integration handoff

- Date: 2026-07-15
- Task: `TASK-260712-2i3u7v`
- Contract: `pulsar.air-lifecycle-policy.v1`

The macOS app now exposes saved Air management as a first-class SwiftUI
section and a menu-bar status/entry point. `AirAppClient` is the only wire
adapter: it uses the authenticated common Air routes, bounded same-origin JSON,
opaque public identifiers, per-mutation idempotency keys and canonical error
codes. `MacAirAppComposition` owns refresh/mutation tasks and maps those core
models into `NodeAppUI`; SwiftUI does not perform network or lifecycle work.

## User-visible lifecycle

The Air screen shows the one active Air separately from saved memberships,
plus aggregate joined/active/online counts, both capacity ceilings, the local
Air role and effective invite/overlay/queue/replace policy. The common API does
not expose member identities in authorized Air projections, so the macOS UI
does not invent a member directory or render opaque IDs. Alias-created
two-party Airs appear by their human Air title and ordinary aggregate state.

Current-primary flows cover create, one-time invite issue/withdraw, invite
consume preview, joining-primary confirm or decline, save-only confirmation,
activate/switch/deactivate, leave, owner dissolve and owner policy replacement.
Switch, deactivate, leave, dissolve and confirm-with-switch state their
main-track/overlay effects and require an explicit confirmation. Owner leave is
not offered because the contract requires ownership transfer or dissolve; the
server remains authoritative for role, revision, capacity and concurrent
active-pointer checks.

Invite input uses a secure field and is discarded after submission. An issued
code is held only in the in-memory shell projection, marked privacy-sensitive,
shown once for explicit copy, and can be hidden or withdrawn. Its clipboard
copy clears after 60 seconds only when it still owns the pasteboard value, so a
newer user copy is preserved. Codes, Air IDs and membership IDs never become
labels, accessibility text, logs or durable app state.

## Accessibility, errors and dependency boundary

The section uses native `List`/`Button`/`Picker`/`DisclosureGroup` semantics,
stable Air identity, keyboard shortcuts and explicit VoiceOver labels/groups.
English and Russian copy maps stale/unavailable invites, role denial, both
capacity ceilings, revision/active-pointer conflicts, owner-transfer
requirements and offline/runtime-barrier failures without claiming success.

This integration depends only on the frozen Air control-plane and the existing
Phase 1 shell/authentication composition. It does not add target discovery,
target snapshots, inbox/history pagination, public Air discovery or streamed
track assumptions; those remain owned by their later Phase 2 stories.

## Automated evidence

- `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test`
  passes all 221 tests, including every Air lifecycle route, canonical failures,
  inconsistent-current rejection, opaque-label source guards, action forwarding
  and disruptive-confirmation/accessibility seams.
- `swift build` passes with the Command Line Tools compiler; the pre-existing
  `PlayerCore` Swift 6 Sendable warnings remain unrelated to this task.
