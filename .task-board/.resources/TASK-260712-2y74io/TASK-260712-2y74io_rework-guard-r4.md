# TASK-260712-2y74io — root review round 4 / mandatory R5 guard

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **REWORK — R4 is not accepted**

This guard is cumulative with the accepted Rev16 bridge, root R1–R3 guards,
and root W1–W4 directives. Preserve the dirty worktree; do not commit or push.

## R4-F12 — confirmed shutdown can block behind ordinary lifecycle work (HIGH)

`confirmedShutdownAdapter.confirm` calls `lifecycle.beginLifecycle` from the
`WM_ENDSESSION` wndproc before publishing the abrupt latch. The production
`lifecycleTracker` deliberately holds `lifecycle.mu` while invoking external
work in `runGatedWork`, `runRearm`, `runPermissionRequest`,
`runCapturePrepare`, and `runCaptureActivation`. Therefore an already-admitted
helper/UI callback can hold that mutex indefinitely and make the confirmed
shutdown wndproc wait for it. This contradicts R3-F10: confirmation may
classify in-flight ordinary work as pre-confirmation, but the wndproc must
never wait for that work and must return promptly.

Required correction:

1. The confirmed path must perform its exact current-native-owner stop,
   monotonic latch, wake, and return without acquiring any mutex that is held
   across ordinary external work. Do not move the wait to another ordinary
   mutex or spin/retry in the wndproc.
2. Publish the active capture generation/operation through a race-safe,
   nonblocking production ownership seam (atomic immutable snapshot or an
   equivalent design). The snapshot must not pair an operation ID with the
   wrong generation and must be cleared only by the matching owner.
3. Close the post-confirmation start/continuation boundary without waiting.
   An external prepare/activate callback admitted before confirmation may
   return, but on return it must observe the abrupt latch, suppress evidence/UI
   continuation, and nonblockingly stop any native operation it created. It
   must not activate a newly prepared operation after confirmation.
4. A native capture already published at confirmation receives exactly one
   shutdown stop request before the latch wake. No lifecycle evidence,
   unregister, result take, release, artifact finalization/abort, sync,
   cleanup, or helper destruction is added to the wndproc.
5. Preserve W1–W4: both wndproc entry guards, waiter priority and bounded
   no-sync partial append, evidence enqueue/physical-I/O suppression,
   post-pump error/UI suppression, late quit/watchdog gating, and deferred
   cleanup suppression.

Required executable evidence:

- Hold the real production `lifecycleTracker` inside each external-callback
  class (at minimum prepare and activation) with a barrier. Invoke the same
  adapter used by `WM_ENDSESSION` concurrently and prove confirmation returns
  and wake occurs before releasing the callback.
- Assert shutdown stop precedes latch/wake for a published exact generation;
  no wrong/stale operation is stopped.
- For a prepare that returns a new operation only after confirmation, prove it
  is stopped/cancelled and never activated, logged, posted, released, or
  finalized. For an activation already admitted before confirmation, prove no
  successor work begins.
- Repeat the schedules under `-race`; source-string assertions alone do not
  count as production-seam evidence.

After the final edit rerun focused schedules repeatedly, full uncached tests
and race, vet, Windows amd64/arm64 build plus test compilation, manifest/privacy
checks, formatting, `git diff --check`, and `task-board validate`. Refresh the
R5 outcome with exact hashes and honest signed-Windows hardware gaps. A fresh
independent reviewer and root line-by-line audit remain mandatory.
