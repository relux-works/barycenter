## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:37:28Z

## Last Update
2026-07-12T16:47:20Z

## Blocked By
- TASK-260712-3sj8ox
- TASK-260712-3sv87k
- TASK-260712-1kk8bd
- STORY-260712-25lysg
- STORY-260712-fes2jj
- STORY-260712-34kbkn
- STORY-260712-3v14m9
- STORY-260712-ob1tx2

## Blocks
- TASK-260712-11e4e3
- TASK-260712-2f0gpu
- TASK-260712-uht9e2

## Checklist
- [ ] Evaluate timezone and quiet-hour rules consistently for schedules and API triggers.
- [ ] Resolve targets through explicit ACL snapshots and Air policy before creating any transmission.
- [ ] Enforce immediate revoke, disable, dedupe, cancel, and runaway rate limits in the runtime.
- [ ] Reuse cue delivery and mixer seams without opening microphone or bypassing recipient controls.

## Notes
Root-reviewed invariant: coordinator policy selects eligible transmissions, but the recipient mixer alone enforces its local output ceiling last. Tests must verify this seam; runtime must not duplicate or override it.

## Precondition Resources
(none)

## Outcome Resources
- [p3-soundboard-automation-components.puml](file://TASK-260712-1eva0y/p3-soundboard-automation-components.puml) — Component diagram for the coordinator automation runtime and safety checks
- [p3-soundboard-automation-sequence.puml](file://TASK-260712-1eva0y/p3-soundboard-automation-sequence.puml) — Sequence diagram for cue trigger execution, policy checks, revoke, and quick disable
