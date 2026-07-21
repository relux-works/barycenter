## Status
backlog

## Review
required

## Task Class
metadata

## Blocked By
- TASK-260721-2346wf

## Blocks
- (none)

## Checklist
- [ ] Install and launch exact Windows and macOS candidates as ordinary GUI applications.
- [ ] Exercise deny/allow, built-in-microphone capture, playback, interrupt and recovery on both platforms.
- [ ] Spot-check Windows scaling/keyboard and macOS Retina/keyboard/VoiceOver presentation.
- [ ] Exercise one real routed transport plus enabled stream/live capability without duplicate playback.
- [ ] Smoke moderation, delete, recovery, restart and final cleanup.
- [ ] Complete the handoff-defined passive soak and return one timestamped verdict packet.

## Notes
Owner: Ivan Oparin. Do not start until TASK-260721-2346wf Desktop UI automated acceptance and owner handoff publishes exact candidate hashes and the autonomous engineering goal is otherwise complete.
Autonomous handoff candidate: source a7258db, local manifest SHA-256 17735d6f42371e75824689bcdc926676bb1b29dd2f63cf4dd7e897e126a6970b, hosted desktop jobs in CI 29828660852 passed, signed diagnostic probe SHA-256 42081733678469a97d065ef0c0950c7f481b3e63ccfc5b528dce96a50fac8994. Do not start production rows until a signed Windows production candidate and notarized macOS DMG from this accepted source are supplied. The attached final-owner-verification.md is the only manual checklist; no terminal use or legacy task revival is requested.

## Precondition Resources
- [final-owner-verification.md](file://TASK-260721-ryk8c0/final-owner-verification.md) — Single no-terminal Windows and macOS real-app checklist with exact engineering hashes, honest release-artifact wait boundary and result packet

## Outcome Resources
(none)

## Created
2026-07-21T10:58:43Z

## Last Update
2026-07-21T12:10:44Z

## Assigned To
Ivan Oparin
