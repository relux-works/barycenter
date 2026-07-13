# macOS local self-test and short-file intake

## Description
Implement the exact offline self-test and short-file draft flow on top of the macOS capture and clip-output services.

## Scope
Provide Try locally with two explicit actions: play the reviewed builtin cue, or record exactly five seconds and then play that completed recording through the same clip mixer and selected output used by network media. The self-test must make no coordinator, upload, or telemetry call and delete its draft on close or explicit delete. Add the system file picker and drag-drop, show filename, detected format, duration, size, audience, eligible delivery modes and the rights reminder, and reject files beyond phase-one limits with honest phase-two guidance while leaving server probing authoritative.

## Acceptance Criteria
A clean unpaired macOS install can play the builtin cue and can record five seconds then hear the completed recording through the production clip branch; captured audio is not monitored live and no network request occurs; the draft is removed on close or delete. Supported short files produce a reviewable draft, while unsupported or over-limit files are never shown as accepted.
