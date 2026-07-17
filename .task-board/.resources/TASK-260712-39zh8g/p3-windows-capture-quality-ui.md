# P3 Windows capture-quality UI engineering handoff

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-39zh8g`

Status: best-effort UI engineering complete; real-app accessibility, DPI and hardware checks not run

## Local ownership and presentation

The Windows shell now owns capture-quality configuration in the existing local
capture workflow and mirrors its validated state into both the main Win32
window and notification-area menu. The coordinator can neither select a mode,
grant degraded consent, start capture, nor suppress a degraded result.

Settings and the local self-test expose `auto`, `speaker`, and `headphone`
selection. The textual projection shows requested and resolved route,
lifecycle, AEC, noise suppression, input AGC, input health, and the two distinct
fixed contract ceilings: `-3 dBFS` for capture-input AGC and `-1 dBFS` for the
receiver post-mix output. Receiver-local volume remains separate and cannot
raise microphone gain.

The current packaged native helper selects the Windows communications category
but does not constitute proof of native AEC or noise suppression. Consequently,
preflight remains `degraded/aec_unavailable`, never accepted. An explicit local
toggle grants degraded capture for one subsequent generation; terminal cleanup
clears that consent. Mode and consent controls are disabled while capture is
active.

## Visible stop and failure handling

Preparing, awaiting-consent, capturing, and reconfiguring lifecycles replace
the main window's ordinary Record button with a persistent native Stop button.
The foreground Escape command and Ctrl-period invoke that same local workflow
cancel seam. The notification-area menu shows quality, lifecycle, route and
both ceilings, and exposes local Stop while active.

English and Russian copy distinguishes accepted, degraded and unsupported
quality, as well as active, not-required, unavailable and faulted effects. It
provides exact guidance for permission denial, missing device, missing/stale
reference, unknown/excluded route, unavailable effects, silent/quiet/clipping
input, clock instability, processor overrun, device loss, re-arm timeout,
unprocessed fallback and mixed versions. Failed generation state remains
visible until the next local attempt.

## Production boundary

Recorded clips and the five-second self-test use this configuration through
`WindowsCaptureWorkflowController`. The generic presentation also understands
`live_ptt` states, but shipping `main.go` still does not construct
`WindowsLivePTTNode` and does not advertise its capability. This task therefore
does not claim production live-PTT integration or a successful microphone
result. UI presence is not a C3 acoustic claim.

## Automated and manual evidence boundary

Go tests cover fail-closed preflight, mode cycling, exact route/effect/health
projection, distinct ceilings, one-generation consent cleanup, failed-state
retention, localized typed failures, native Win32 controls, Ctrl-period,
foreground Escape, tray Stop, DPI-aware layout source seams, and absence of a
remote-start/live-node production seam. The Windows amd64 CGO-free cross-build
also completes.

No real application was operated for this engineering task. Narrator reading
order, Tab traversal in a signed MSIX, layout across supported DPI values,
Windows permission prompts, device reconnect, physical route changes, acoustic
behavior, and immediate Stop latency are `not-run` and remain in manual epic
`EPIC-260714-th54l3`.
