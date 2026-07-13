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
- TASK-260712-1g6lk8
- TASK-260712-3d6cnn
- TASK-260712-1ckdr7

## Checklist
- [ ] Move clip preparation and scheduling out of the WASAPI render callback
- [ ] Implement additive main plus clip mixing with continuous ring consumption and shared duck parameters
- [ ] Add limiter and overlay telemetry without reintroducing render-thread locks or stale state
- [ ] Implement the exact gain order, default duck ramps, pre-duck timing and final local ceiling
- [ ] Ramp cancellation safely and keep main-ring consumption continuous through every overlay state

## Notes

## Precondition Resources
(none)

## Outcome Resources
(none)
