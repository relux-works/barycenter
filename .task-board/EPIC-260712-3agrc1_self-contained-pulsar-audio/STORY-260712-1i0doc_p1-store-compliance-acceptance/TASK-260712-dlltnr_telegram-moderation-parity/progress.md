## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:45:04Z

## Last Update
2026-07-15T07:25:25Z

## Blocked By
- TASK-260712-1epb3a
- TASK-260712-2kec2s
- TASK-260712-3e4p0c
- TASK-260712-3dmllz
- TASK-260712-1x0lot

## Blocks
- TASK-260712-2s4e9p
- TASK-260712-1xik11

## Checklist
- [x] Define which Telegram commands, inline actions or history views expose report, block or delete in phase one.
- [x] Reuse the same moderation terminology and reporter safe status mapping as the app surfaces.
- [x] Verify that legacy Telegram voice order, defaults and receipt labels do not regress when moderation actions are added.
- [x] Add integration coverage for Telegram and shared history moderation paths changed by this task.
- [x] Use actor-bound expiring callbacks and canonical history action capabilities
- [x] Test forged, group, cross-user, repeated and expired moderation actions

## Notes
2026-07-15 strict inline kickoff from synchronized main merge 1c45953 after accepted macOS UGC controls. Reuse the canonical moderation/history action services and frozen six-reason/outcome vocabulary; preserve secure actor-bound callbacks and legacy Telegram delivery defaults. Live Telegram-client/manual observations remain in the separate manual-test epic; this engineering task covers best-effort code, unit/integration tests and CI.
Engineering commit 8ce1b8c adds private /history replay delete report and sender-block controls through canonical history media moderation and policy services; opaque actor/chat/message/action-bound 15-minute callbacks; shared current-installation receipt mapping; exact RU/EN copy; and rollback-safe moderation target schema migration. Clean exact-head all-suite acceptance task-260712-dlltnr-engineering passed 12/12 with start/end dirty false at full head 8ce1b8cf4ced1840a555c8356b45754e405d21df. Coordinator full tests, vet and relevant race suites pass. Live Bot API and real-device observations remain manual in EPIC-260714-th54l3. Awaiting hosted PR CI before done.
Hosted PR run 29397089442 passed all four jobs on tracking head 88de480: coordinator including exact previous-head rollback, authoritative Xcode Swift, Windows portable/race/cross-build, and signed packaged probe. Engineering scope accepted. No live Telegram client or physical-app result is claimed; those remain manual in EPIC-260714-th54l3.

## Precondition Resources
(none)

## Outcome Resources
(none)
