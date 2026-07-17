# P3 Windows automation administration

Task: `TASK-260712-89fzlc`

The Windows shell now has a dedicated Automation section backed by the
authenticated control API. It lists feature policy, schedules, redacted
principal metadata and display-safe automation history. A lost or insufficient
control identity clears the entire administration projection, so an
unauthorized installation cannot infer whether schedules or principals exist.

## Schedule and policy editing

The editor accepts a display name, an IANA timezone, comma-separated English
weekday abbreviations, local `HH:MM`, and optional weekly quiet windows such as
`Mon 22:00-06:00; Tue 12:00-13:00`. It can create a disarmed schedule, replace
the selected schedule with its CAS revision, enable/disable it, or delete it.
Changing the feature timezone/quiet policy uses the feature CAS revision and
therefore retains the coordinator's existing rule that policy changes disarm
stale schedules.

The next-run projection enumerates UTC minutes, matching the coordinator
scheduler. A spring-forward wall minute has no mapping; for a fall-back fold,
the earliest UTC mapping is shown. The UI also says when the next occurrence
will be denied by global or schedule-specific quiet hours. It does not promise
catch-up execution.

## Principals, history, and emergency stop

Issuance is a two-step action and creates a minimal principal scoped to one
saved cue, own-Barycenter overlay delivery, one target, and a 30-day expiry.
The returned 256-bit secret is held only in private process memory. It is never
placed in `ShellSnapshot`, preferences, logs, labels, errors, or an EDIT
control. Explicit copy reuses the hardened Windows clipboard publication with
history/cloud exclusion and compare-and-clear; Hide destroys the in-memory
copy. Later list responses retain only display name, scope counts, expiry, and
revocation state.

Schedule disable/delete, principal issue/revoke, automation disable, pending
history cancellation, and orbit emergency disable require an explicit second
confirmation. History keeps display-safe attribution. Disabling automation
does not change `soundboard_enabled`, remove the Soundboard section, or alter
manual audio actions; those remain separate coordinator and client paths.

## Automated evidence and limits

Go tests cover strict request/response validation, CAS/idempotency headers,
one-time and replayed issuance, secret redaction, unauthorized clearing,
weekly quiet-window parsing, DST fold ordering, quiet-hour explanation,
two-step actions, pending cancellation, and manual Soundboard availability.
The portable shell tests pin the Win32 navigation/control/source seams, and
the Windows cross-build compiles the native path.

This is best-effort repository evidence. It does not claim real packaged
keyboard navigation, screen-reader output, clipboard behavior, signed-binary
interaction, physical DST transition behavior, audible playback, or Windows
hardware results. Those checks remain in the dedicated manual-test epic
`EPIC-260714-th54l3`.
