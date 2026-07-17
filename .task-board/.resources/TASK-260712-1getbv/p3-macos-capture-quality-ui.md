# P3 macOS capture-quality UI engineering handoff

Date: 2026-07-17

Owner: Ivan Oparin

Task: `TASK-260712-1getbv`

Status: best-effort UI engineering complete; real-app accessibility and hardware checks not run

## Local ownership and presentation

The macOS shell now mirrors the shared capture-quality state without importing
`NodeCore` into `NodeAppUI`. The production composition is the only adapter: it
maps the validated local backend state into a presentation-only model and keeps
the existing telemetry forwarding intact. The coordinator cannot select a
capture mode, grant degraded consent, start the microphone, or hide a degraded
result.

Settings and the local self-test expose `auto`, `speaker`, and `headphone`
selection. The UI shows the resolved route, lifecycle, AEC, noise suppression,
input AGC, input health, and distinct contract ceilings: the fixed `-3 dBFS`
capture-input safety ceiling and the separate fixed `-1 dBFS` receiver
post-mix ceiling. These values are deliberately read-only contract facts;
recipient-local volume remains a separate existing control and changes neither
ceiling.

Speaker preflight is never described as accepted before a proven render
reference exists. It shows `degraded/reference_unavailable` and offers an
explicit local consent toggle. Consent applies to one subsequent local capture
generation and is cleared when that generation ends. Mode and consent controls
are disabled while capture is active.

## Visible stop and failure handling

An active quality lifecycle, normal clip recording/processing, or local
self-test permission/recording phase adds a persistent bottom status bar in the
main window. The bar uses a native button, textual status and a non-color symbol;
Command-period and the foreground Escape command invoke the same immediate
local Stop seam. The menu bar also shows current quality, reason, both ceilings,
and the local Stop command while active.

English and Russian copy distinguishes accepted, degraded, unsupported,
effect-active, effect-not-required, effect-unavailable, and effect-faulted
states. It includes explicit guidance for permission denial, missing device,
missing or stale reference, unknown or excluded route, unavailable effects,
quiet/clipping input, clock instability, processor overrun, device loss,
re-arm timeout, user-selected unprocessed fallback, and mixed versions. A
connection-state change does not erase the local quality generation or Stop
authority.

## Workflow boundary

Recorded clips and the five-second local self-test are connected to this shell
surface in the production `NodeApp` composition. The shared presentation model
also renders `live_ptt` generations and lifecycle state, but the production app
still intentionally does not instantiate `MacLivePTTNode`; therefore this task
does not claim a live-PTT UI integration or live microphone result. That
production-dark boundary remains covered by existing source contracts and later
integration work.

`capture_quality_v1` remains unadvertised. UI availability is not a capability
or C3 acoustic claim.

## Automated and manual evidence boundary

Swift tests cover localized catalog completeness, route/mode and fixed-ceiling
projection, stale-state reset, reconnect retention, clip and live lifecycle
presentation, all local action forwarding, native Button and keyboard seams,
accessibility labels/values/hints, menu Stop wiring, and absence of coordinator
or runtime start authority in the capture action slice.

No real application was operated for this engineering task. VoiceOver reading
order, keyboard traversal in a signed app, visual layout at supported window
sizes, TCC prompts, physical route changes, acoustic behavior, and immediate
stop latency are `not-run` and remain in manual epic `EPIC-260714-th54l3`.

