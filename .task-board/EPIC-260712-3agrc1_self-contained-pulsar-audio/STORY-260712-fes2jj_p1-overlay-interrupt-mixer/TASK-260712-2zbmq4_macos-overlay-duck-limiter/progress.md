## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:44:14Z

## Last Update
2026-07-12T16:41:45Z

## Blocked By
- TASK-260712-1hqiek

## Blocks
- TASK-260712-8mwyiv
- TASK-260712-3d6cnn
- TASK-260712-19w1qn

## Checklist
- [ ] Prepare and arm clip playback off the render thread with explicit cache or file-handle lifecycle
- [ ] Mix clip audio additively with pre-duck or release behavior while main program keeps advancing
- [ ] Emit overlay continuity telemetry and verify cancellation or replacement leaves the graph ready for the next clip
- [ ] Implement the exact gain order, default duck ramps, pre-duck timing and final local ceiling
- [ ] Ramp cancellation safely and keep source-ring consumption continuous through every overlay state

## Notes

## Precondition Resources
(none)

## Outcome Resources
(none)
