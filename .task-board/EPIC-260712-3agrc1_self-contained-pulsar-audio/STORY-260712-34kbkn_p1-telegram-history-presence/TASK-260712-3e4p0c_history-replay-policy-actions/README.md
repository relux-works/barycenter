# Expose replay, delete, report and mute actions from history

## Description
Provide one authorization-aware action orchestration layer behind Phase 1 history while delegating storage and moderation ownership to their dedicated services.

## Scope
Return allowed action flags and implement replay, delete, report and mute or block commands for app and Telegram consumers. Replay of ready, unexpired, undeleted media creates a new transmission with a new coordinator acceptance time and newly resolved targets; it never reuses the old snapshot or acts as missed-delivery inbox autoplay. Delete delegates to the owner-policy media lifecycle and cancels pending work. Report delegates to the moderation service. Mute delegates to the block contract. Make every command ActorContext-scoped, idempotent where required and auditable.

## Acceptance Criteria
Only an authorized actor sees or executes each action. Replay cannot fetch or target outside current rights, cannot revive deleted or expired media and is always an explicit new delivery. Delete, report and mute reach exact visible outcomes without duplicating business logic. App and Telegram actions share the same command service and tests cover revoked actors, races and repeated requests.
