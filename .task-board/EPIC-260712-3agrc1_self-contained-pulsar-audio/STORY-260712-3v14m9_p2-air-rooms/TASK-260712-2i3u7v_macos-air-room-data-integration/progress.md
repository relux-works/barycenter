## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:56Z

## Last Update
2026-07-15T14:51:43Z

## Blocked By
- TASK-260712-17yizc
- TASK-260712-2vhf80
- TASK-260712-25862f
- TASK-260712-2bjdlb
- TASK-260712-3dqc3l

## Blocks
- TASK-260712-3nq0tq
- TASK-260712-2nto40

## Checklist
- [x] Integrate current Air and pending join state into macOS read models
- [x] Wire create, join, confirm, leave, and dissolve actions to the new API and stable errors
- [x] Render alias backed two party Airs without raw ids or target snapshot assumptions
- [x] Verify the dependency boundary with the phase one UI shell and the explicit targets story
- [x] Render saved and active Airs and require confirmation for disruptive switch, leave or dissolve
- [x] Verify keyboard and VoiceOver lifecycle flows

## Notes
Strict inline execution started from synchronized main eaf8070 after accepted approach-to-Air tracking merge. SwiftUI expert guidance is active; inspecting the existing macOS shell, control API client, localization and accessibility test seams before designing Air read/action state.
Accepted after engineering commit 13a65d1, PR #94, hosted run 29424982574 green 4/4, and merge 8cd46b1. Local Xcode Swift tests passed 221/221 and swift build passed. Common Air API covers saved/current and pending confirmation plus create, invite, join, confirm, decline, activate, switch, deactivate, leave, dissolve and policy actions. EN/RU keyboard and VoiceOver flows, secure invite redaction, aggregate membership, and explicit no-raw-ID/no-target/no-inbox boundaries are covered.

## Precondition Resources
(none)

## Outcome Resources
- [p2-macos-air-room-data-integration.md](file://TASK-260712-2i3u7v/p2-macos-air-room-data-integration.md) — Accepted implementation handoff for macOS Air room management
