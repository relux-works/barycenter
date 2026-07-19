## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:49:11Z

## Last Update
2026-07-19T20:19:26Z

## Blocked By
- TASK-260712-aniuyy
- TASK-260712-47uve0
- TASK-260712-20j5tm

## Blocks
- TASK-260712-1rziyo
- TASK-260712-28zhpl
- TASK-260712-1u57qz
- TASK-260712-39vjzd
- TASK-260712-2q4jbu

## Checklist
- [ ] Store distinct device group grant and content-key state under DPAPI
- [ ] Implement transactional persist-before-ack and clone or rollback detection
- [ ] Pass known-answer epoch replay fork and crash vectors
- [ ] Redact config logs telemetry crashes and diagnostics
- [ ] Publish narrow send playback live and UX interfaces

## Notes
Owner gate 2026-07-16: moved to EPIC-260716-3qsztl Deferred E2EE media implementation after independent audit. Do not implement or move this task into development until TASK-260712-aniuyy Pass independent cryptographic design review before implementation is done with no open critical or high finding. Any protocol-affecting delta reopens the audit gate.
Upstream TASK-260712-20j5tm independent review follow-up I1 / EPC-005: explicitly pin client semantics for an active Air member whose registered device rows are all revoked. Current coordinator treats those devices as removed endpoints rather than an unsupported target; Windows key-state review must confirm or reject that interpretation.

## Precondition Resources
(none)

## Outcome Resources
(none)
