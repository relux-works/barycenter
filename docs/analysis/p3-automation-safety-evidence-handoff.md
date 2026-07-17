# P3 automation safety engineering evidence handoff

`TASK-260712-2f0gpu` closes the repository-engineering part of C7. It does not
accept C7 on real applications, signed packages, physical clocks or audio
hardware. Final hands-on acceptance remains `TASK-260712-1gyohk` in
`EPIC-260714-th54l3`; the contract records `manualEvidence` as `not-run`.

## Decision

The coordinator, Windows, macOS and Telegram implementations have rerunnable
deterministic coverage for the frozen soundboard and automation safety
contract. Engineering evidence may be handed to the Phase 3 review line, but
automation remains ineligible for production promotion until the manual C7
matrix, independent automation review, rollout/recovery drills and beta gate
are accepted.

The machine-readable authority is
`acceptance/phase3/automation-safety-evidence-v1.json`. Its validator rejects
missing coverage, invented manual results, source drift, promotion claims and
unsafe rollback ordering.

## Repository evidence matrix

| Safety property | Deterministic evidence | Result boundary |
|---|---|---|
| IANA timezone, DST gap/fold and clock jumps | coordinator occurrence tests plus Windows/macOS calendar rules | spring gaps never fire, earliest fall mapping wins, backward duplicates replay once, forward jumps do not catch up |
| Quiet hours | feature and schedule policy validation plus runtime denial tests | malformed or overlapping policy fails closed; matching minutes create no transmission |
| DND, block and Air policy | `TestAutomationRuntimeAirDNDAndBlockPolicyMatrix` | recipient DND/block remains last-mile authoritative; Air leave and disabled overlay deny before a new transmission |
| Explicit target ACL | target-reference issue, digest and runtime scope tests | raw references are not persisted; foreign, unknown, revoked and out-of-scope targets collapse to the same denial |
| At-most-once, restart and queue bounds | concurrent API/schedule claims, lease recovery, current-minute runtime and rate/concurrency tests | one execution/transmission per authority key; no missed-minute autoplay; retained attempts are bounded and pruned |
| Revoke, disable and cancel | principal revoke, schedule/feature disable, pending cancel and ordinary scheduler cancellation tests | new claims fail immediately; already accepted pending work enters the canonical cancellation path |
| Recipient local ceiling | accepted P1 Windows and macOS mixer tests | cue/overlay is summed before the limiter and final local master gain; coordinator automation cannot raise the recipient ceiling |
| No microphone | Windows/macOS soundboard source tests and client request tests | cue CRUD, manual trigger and automation administration do not import or invoke capture services |
| Surface reconciliation | Windows/macOS admin tests and Telegram opaque callback tests | UI/bot actions use current role/revision/policy, keep one-time secrets out of projections and reconcile through canonical history |
| Manual soundboard independence | feature-state and both desktop composition tests | disabling automation does not disable or reroute manual saved-cue delivery |

## Operator handoff

Deploy only additively and dark: coordinator/schema first, then clients and
Telegram adapters. Do not issue scoped principals or enable schedules until
the reviewed coordinator and every target client are deployed. Enabling is an
orbit-scoped control action with the current feature revision; there is no
global production enable claim in this handoff.

For an automation incident, use this order:

1. set orbit automation disabled or emergency-disabled with the current
   revision;
2. revoke every affected scoped principal and disable affected schedules;
3. run the canonical invalid-automation cancellation reconciliation until no
   candidates remain;
4. keep manual soundboard and unrelated audio paths independently observable;
5. retain automation tables, immutable audit/history and idempotency rows;
6. deploy a retained predecessor only after automation callers are withdrawn
   and pending work is terminal.

Never delete additive automation tables during rollback. A replay of an old
idempotency key must remain the old result, not create replacement work. A
missed scheduled minute is terminally missed and must not autoplay after
restart or clock correction.

## Exact remaining matrix gaps

Repository CI cannot establish any of the following, and the evidence contract
records each as `not-run`:

- signed Windows and signed macOS application interaction;
- physical forward/backward clock and timezone manipulation;
- audible DND/block/Air-leave behavior and recipient ceiling quality;
- Windows keyboard/Narrator and macOS Full Keyboard Access/VoiceOver behavior;
- live Telegram delivery and stale callback behavior against the production
  bot transport;
- real clipboard-manager observation, screenshots and secret-leak inspection;
- kill-switch wall-clock latency on real clients, real hardware, mixed fleets,
  rollout/recovery drills and the seven-day beta.

These gaps are owned by `TASK-260712-1gyohk`, with rollout/recovery and soak
continuing in `TASK-260712-30xwu2` and `TASK-260712-1actom`. None is inferred
from a build, unit test, signed package probe or source inspection.
