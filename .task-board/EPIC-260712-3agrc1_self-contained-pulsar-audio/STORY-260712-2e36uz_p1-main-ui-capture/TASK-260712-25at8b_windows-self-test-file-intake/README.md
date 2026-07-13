# Windows local self-test and short-file intake

## Description
Implement the exact offline self-test and brokered short-file draft flow on top of the Windows capture and clip-output services.

## Scope
Provide Try locally with two explicit actions: play the reviewed builtin cue, or record exactly five seconds and then play that completed recording through the same clip mixer and selected output used by network media. The self-test must make no coordinator, upload, or telemetry call and must delete its draft on close or explicit delete. Add the standard brokered file picker and drag-drop, show filename, detected format, duration, size, audience, eligible delivery modes and the rights reminder, and reject files beyond phase-one limits with honest phase-two guidance while leaving server probing authoritative.

## Acceptance Criteria
A clean unpaired Windows install can play the builtin cue and can record five seconds then hear the completed recording through the production clip branch; captured audio is not monitored live and no network request occurs; the local file is removed on close or delete. Supported short files produce a reviewable local draft, while unsupported or over-limit files are never presented as accepted and do not require broad filesystem capability.
