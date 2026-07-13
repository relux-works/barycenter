# Expose capture quality and diagnostics in macOS

## Description
Render the shared capture-quality state and safe controls in the macOS main window and menu bar.

## Scope
Add speaker, headphone and auto selection, separate input AGC ceiling and receiver output-volume ceiling controls, pre-capture capability warning, active capture indicator, degraded or unsupported reason, input too quiet or clipping guidance, device or reference loss, and an immediate local Stop. Make keyboard and VoiceOver semantics explicit, survive route changes and reconnect and never let coordinator state enable capture or hide a degraded speaker path.

## Acceptance Criteria
The macOS UI makes mode, both ceilings, capture state and every accepted, degraded or unsupported reason unambiguous before and during clips and PTT. Permission, device, reference, route and mixed-version failures preserve file playback and safe fallback, accessibility checks pass and no hidden capture or false AEC claim is possible.
