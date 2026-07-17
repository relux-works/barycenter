# Phase 3 automation safety technical pre-review

- Date: 2026-07-17
- Original task: `TASK-260712-1x5jfo`
- Root-reviewed source: `d94f51644a3acf37601b4a869b4247380372f9ec`
- Root-reviewed tree: `4e4cca878db806650eda6f1e1642051b87a18b93`
- Engineering reviewer: `codex-inline-pre-reviewer`
- Independent approval: `TASK-260717-1pyg62`, owner Ivan Oparin

## Decision

The repository technical pre-review is complete. No new Critical or High code
finding was found in the frozen automation and soundboard paths. Reversible
strict-sequence engineering may move to `TASK-260712-7ng1vs`.

This is deliberately **not** an independent acceptance. The same inline
execution chain implemented parts of the reviewed paths, and signed C7 real-app
evidence has not run. Automation activation, C7 acceptance and Phase 3
promotion remain blocked. The fail-closed machine record is
`acceptance/phase3/automation-technical-pre-review-v1.json`.

## Reviewed seams

| Seam | Repository conclusion |
| --- | --- |
| Authority and ACL | Every control mutation and trigger resolves the current actor/orbit, content-policy acceptance, cue ownership, scoped audience and current target reference. Foreign and unknown targets collapse to denial. |
| Principal secrets | A principal exposes one random secret once, persists only a domain-separated hash, has bounded scope/lifetime and is rejected immediately after revoke, disable, expiry or issuer removal. |
| Idempotency | Request digest conflicts are terminal denials. Replays return the original execution and never create replacement transmission work. |
| Schedules and clocks | Only the current canonical minute is eligible. Spring gaps never fire, the earliest fall-fold instant wins, backward jumps replay the same occurrence and forward jumps never catch up. |
| Runtime bounds | Principal/minute, orbit/hour, per-principal and per-orbit concurrency, lease duration, retry generation, candidate batches and retention pruning are bounded. |
| Recipient policy | DND, block, Air membership, target generation, online/capability and overlay delivery are rechecked before the ordinary transmission is accepted. |
| Kill switches | Feature disable, emergency disable, schedule disable, principal revoke, issuer revoke and cue deletion enter canonical cancellation and remain idempotent. Additive rows and audit history survive rollback. |
| Telegram | Callback tokens are opaque, chat/message/actor-bound, expiring and one-shot. Execution rechecks live revisions and authority; crash after claim loses the action rather than firing twice. |
| Audio boundary | Automation cannot choose a microphone path. Saved cues use the ordinary overlay mixer and cannot raise the recipient's local post-limiter ceiling. |

## Reproduced evidence

```text
(cd coordinator && go test -race -count=10 ./internal/store -run '^(TestAutomationRuntimeAirDNDAndBlockPolicyMatrix|TestAutomationRuntimeAtomicallyCreatesOneTransmissionAndReplays|TestAutomationRuntimeBoundsAttemptsConcurrencyAndPruning|TestAutomationRuntimeEnforcesPrincipalAndOrbitConcurrencyCaps|TestAutomationRuntimeRevokeCancelsOrdinarySchedulerWork|TestAutomationRuntimeScheduleUsesOnlyCurrentCanonicalMinute|TestAutomationScheduleDSTGapFoldClockJumpAndConcurrentClaims|TestAutomationClaimAndLeaseCrashBoundariesReconcile|TestAutomationQuickDisableAndScheduleDisableExposePendingCancellation|TestAutomationControlForeignAndUnknownTargetReferencesCollapse)$' && go test -race -count=10 ./cmd/duet-coordinator -run 'Automation|Soundboard|TelegramSchedule' && go test -tags previoushead -count=10 ./internal/store -run '^TestAutomationExactPreviousHeadRollback$')
(cd pulsar-win && go test -race -count=10 ./... -run 'Automation|Soundboard|OverlayContinuouslyConsumesMain')
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter 'Automation|Soundboard|Overlay'
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v scripts/acceptance/test_automation_safety_handoff.py
python3 scripts/acceptance/validate_automation_safety_handoff.py
```

The Windows run passed four packages under race and ten repetitions. The Swift
run passed 19 tests in three suites on the available x86_64 macOS test host.
The existing fail-closed handoff passed four contract tests. The coordinator
adversarial, scheduler, callback and exact previous-head groups passed ten
repetitions, including race coverage for current implementation paths.

An earlier broad expression selected 54 store tests and attempted 540 race
executions. It exceeded Go's default ten-minute package timeout while an
unrelated transmission scheduler test was running, so it is recorded as
`timeout-not-counted`, not passing evidence and not an automation finding. The
frozen ten-test adversarial group then passed race×10 in 153.875 seconds; the
coordinator/Telegram group passed race×10 in 69.780 seconds; exact previous-head
automation rollback passed ten repetitions in 58.658 seconds.

## Evidence boundary and external closure

Repository evidence covers deterministic authority, idempotency, DST and clock
rules, bounded rate/concurrency/lease state, policy rechecks, no-microphone
composition and rollback preservation. It cannot prove signed desktop
interaction, physical clock changes, audible DND/block/Air/ceiling behavior,
live Telegram callbacks, real-client kill-switch latency, accessibility or
clipboard/screenshot secret observation.

Those checks remain in manual C7 task `TASK-260712-1gyohk` under
`EPIC-260714-th54l3`. Ivan Oparin must select a reviewer who implemented none of
the paths and close `TASK-260717-1pyg62` against the exact root-reviewed commit
and signed C7 artifacts. Every Critical or High finding must be fixed and
independently re-reviewed. Any affected source, schedule contract, fixture,
build or runtime-config delta reopens root and automation review.
