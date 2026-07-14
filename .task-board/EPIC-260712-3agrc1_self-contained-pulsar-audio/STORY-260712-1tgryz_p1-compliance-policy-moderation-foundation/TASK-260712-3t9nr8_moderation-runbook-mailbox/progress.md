## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:45:04Z

## Last Update
2026-07-14T16:35:34Z

## Blocked By
- TASK-260712-1epb3a
- TASK-260712-2kec2s
- TASK-260712-16zfvu
- TASK-260712-1x0lot

## Blocks
- TASK-260712-2s4e9p
- TASK-260712-1xik11

## Checklist
- [x] Document mailbox ownership, intake template and escalation rules for abuse and Microsoft requests.
- [x] Map each moderation decision to the concrete API or CLI action, audit record and reporter visible status.
- [x] Define evidence copy, retention, privacy and backup expectations for operators handling user audio.
- [x] Add rollback or recovery notes for mistaken delete or disable actions and unsupported requests.
- [ ] Create or verify the real mailbox, accountable rotation and operator credential lifecycle
- [x] Document verified Microsoft removal handling and irreversible versus reversible action policy

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge 867f3d2b262ceed67a7bd75cb90d25bd91c5bcfb after TASK-260712-1x0lot acceptance. Approved defaults: owner and approver Ivan Oparin; support@barycenter.live, moderator@barycenter.live and moderation-urgent@barycenter.live; GMT+4 operations. Reversible repository work, unit tests and deterministic checks proceed. Actual mailbox delivery, human rotation acknowledgment or external Microsoft request receipt will not be invented; any irreversible owner decision is recorded under EPIC-260714-zmnd4n without stopping safe best-effort engineering.
2026-07-14 engineering checkpoint: added report-scoped content-free audit export under list authority, complete operator runbook, machine-readable operations validator, CI contract check and Store-submit mailbox-ready gate. Full coordinator vet/tests, targeted moderation race, exact previous-head moderation rollback, Windows vet/tests/cross-build, Swift release build, JSON and board validation pass. Broad race passed internal/store but an unrelated Telegram test hit SQLITE_BUSY; the changed ModerationHTTP suite passes independently under race. DNS has no barycenter.live MX, so mailbox delivery remains honestly external_action_required and is rerouted to TASK-260714-200ib8 under EPIC-260714-zmnd4n; provider action does not stop reversible engineering.

## Precondition Resources
(none)

## Outcome Resources
- [moderation-operations-runbook.md](file://TASK-260712-3t9nr8/moderation-operations-runbook.md) — Mailbox intake, rotation, least-privilege commands, Microsoft verification, evidence handling, decisions, recovery and incident runbook
- [moderation-operations.json](file://TASK-260712-3t9nr8/moderation-operations.json) — Machine-readable approved operations state and fail-closed external mailbox readiness contract
