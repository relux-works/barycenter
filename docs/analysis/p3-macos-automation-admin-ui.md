# P3 macOS automation administration UI

`TASK-260712-1oodka` adds the authorized macOS control surface for automation
without changing the manual soundboard delivery path. The implementation is
best-effort engineering and automated verification; it does not claim a real
signed-app VoiceOver, keyboard, clipboard, audible-output, DST or hardware run.

## Control and data flow

- `AutomationAdminClient` binds the existing control capability to the frozen
  feature, schedule, scoped-principal and pending-history APIs. Mutations carry
  an idempotency key and the coordinator revision required by the operation.
- `MacAutomationAdminAppComposition` fetches feature state, cues, schedules,
  principals and attributed history as one authorization projection. If any
  refresh fails, it clears the entire projection and one-time credential before
  publishing an unavailable state; an unauthorized UI cannot retain or infer
  configuration from a previous role.
- `PulsarAutomationState` contains only display-safe metadata. The one-time
  credential is held by a private `AutomationPrincipalSecret` reference in the
  app composition and never enters the shell snapshot, view state, preferences,
  logs, errors, screenshots or accessibility values.
- The main SwiftUI sidebar and status menu expose Automation separately from
  Soundboard. Disabling automation does not disable or reroute manual cue
  delivery; the admin screen always offers an explicit path back to Soundboard.

## Schedule semantics

The editor accepts an IANA timezone, `Sun,Mon,...` weekday list, `HH:MM` local
time and optional quiet windows such as `Mon 22:00-07:00`. UTC-minute
enumeration matches the coordinator runtime:

- a local time in a spring DST gap has no fire;
- the earliest UTC mapping wins during a repeated fall DST minute;
- orbit and schedule quiet windows are projected together and a matching next
  candidate is labelled as skipped;
- a newly created schedule remains disarmed until an explicit confirmed enable.

## Destructive and credential actions

Schedule enable/disable/delete, principal issue/revoke, automation toggle,
emergency disable and pending-history cancel require a confirmation dialog.
The coordinator still re-checks current role, revision, policy and target scope.
Principal issuance defaults to the first visible saved cue, own-barycenter
audience, one target and a 30-day expiry.

The credential UI displays only a neutral availability indicator. Explicit copy
places it on the macOS pasteboard for at most 60 seconds. Cleanup compares both
pasteboard change count and payload before clearing, so it never destroys newer
user clipboard content. Hide retires the in-memory reference and any still-owned
clipboard lease; the credential cannot be redisplayed or recovered.

## Automated evidence and manual boundary

Automated coverage includes authenticated request shapes, revision and
idempotency headers, response validation, one-time issuance redaction, revoke
and cancel paths, DST fold/gap and wrapping quiet-hour rules, complete EN/RU
navigation, display-safe snapshot structure, explicit confirmation/source
seams, native SwiftUI controls and production composition wiring.

Real signed-app interaction, Full Keyboard Access order, VoiceOver speech,
screenshots, observed clipboard-manager behavior, audible output and physical
timezone/hardware behavior remain unclaimed in `EPIC-260714-th54l3`.
