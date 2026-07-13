# TASK-260712-2u1w16 — macOS onboarding rework guard R3

The R2 implementation is rejected. Correct every independent-review finding
and every root finding below as one coherent rework. The frozen Rev15 research,
the R1 implementation guard, and root directives M1-M12 remain authoritative.
Do not weaken an accepted invariant, edit the production onboarding UI, commit,
push, reset, checkout, clean, or mark the task `done`.

## Frozen boundary

Read the complete independent report
`TASK-260712-2u1w16_independent-review-r2.md` (SHA-256
`41478cc49106c11975aa39bd33954d24b683660c11015deeb8813f1ca55011d8`)
and the producer outcome. Every producer-listed source/test hash in that report
was independently verified and is the starting boundary. Recheck all hashes
before editing. Windows/coordinator/Telegram work is concurrently dirty and is
out of scope. Preserve `node-app/Sources/NodeApp/main.swift` byte-for-byte at
its verified R2 hash; the narrow M11 change there is already frozen.

## R3-F1 — unsent matching-active state must fail closed

An `ever_sent=false` pending record can never be accepted as prior promotion
merely because its tuple/token matches active credentials. Valid crash
convergence requires the durable false-to-true send barrier. Both
`resumePending` and `recover` must preserve the impossible/corrupt state and
return a structural storage conflict (or equivalently fail closed), with zero
probe/consume sends and zero pending deletion/replacement.

Add deterministic production-seam tests for both public entry points and prove
active and pending bytes remain unchanged.

## R3-F2 — exact protected schemas and byte-equivalent reconciliation

Versioned credential and pending-recovery payloads must reject unknown keys,
duplicate keys, trailing JSON, noncanonical scalar encodings, and any payload
not represented completely by the accepted schema. Automatic DP/login
reconciliation is allowed only for byte-equivalent protected state, as required
by R1; decoded model equality is insufficient. Read-back verification must use
the exact accepted representation before any source update/delete.

Exercise DP/login and pending copies containing unknown fields, duplicates,
different key order/whitespace, and valid-looking but byte-different encodings.
Every non-byte-equivalent pair must fail closed and preserve both copies.

## R3-F3 — limited context survives promotion/delete crash

A successful authenticated 403 limited promotion may retain prior protected
orbit/role metadata, but its limited context strength must be durably represented.
If active save succeeds and pending deletion fails, restart convergence must
still report `hasLimitedContext=true` without another recovery mutation or
network send. Active promotion/probe with full context must durably report
active. Extend the exact versioned protected schema if needed and keep node
bytes unchanged.

Add the pre-existing orbit/role -> 403 -> active-save -> delete-failure ->
restart schedule, plus full-context and old-bundle migration/validation cases.

## R3-F4 — UTS46 mapping precedes one root-dot removal

Apply the frozen ICU UTS46 operation first, then strip exactly one mapped
trailing root dot, then validate labels and length. Accept the canonical
single-root forms ending in U+002E, U+3002, U+FF0E, and U+FF61. Reject multiple
root dots, empty/internal labels, mapping errors, and overlength results.

## R3-F5 — direct export succeeds only after checked close

`RecoveryExportHelper.save` currently sets success after write+`fsync` while
ignoring `Darwin.close`. A delayed write error reported by close therefore
returns false success. Refactor the direct, no-temp export behind a narrow
injected file-operation seam. Success requires, in order: exclusive no-follow
open, complete write loop (EINTR/zero progress handled), successful `fsync`, and
successful checked close. Any failure must return generic `writeFailed`, attempt
safe truncate/remove cleanup without masking the original failure, and never
claim success or leave a known partial file silently. Never retry `close` after
it returns an error because descriptor ownership is then unspecified.

Add deterministic short-write, zero-write, write-error, fsync-error, close-error,
and cleanup-error tests against the actual production save algorithm. Assert no
temp/sidecar/recent-document path and no secret in errors or filenames.

## R3-F6 — crash-friendly descriptions are value-independent

`NodeCapability.description` prints public initializer input `slot` verbatim.
That input is not structurally validated at construction and can contain a
secret canary. Make ordinary/debug descriptions independent of token, slot,
WebSocket URL, coordinator origin, recovery ID/secret, codes, and arbitrary
unvalidated strings. Keep only fixed labels and safe booleans/counts (or redact
the whole capability). Add malicious direct-initializer and reflection/canary
tests, not only valid decoded-bundle tests.

## R3-F7 — clipboard clear errors retain the exact lease

Expiry and explicit clear currently erase local lease ownership before
`clearIfUnchanged`; a transient error can leave the recovery secret on the
system pasteboard forever with no timer or retry authority. Do not retire the
exact lease on an error. Retire it only after atomic compare-and-clear succeeds
or the same atomic operation proves the clipboard was replaced. Use a
testable, bounded/capped retry schedule with no tight loop and keep old-timer
and competing-copy identity exact. Explicit clear must surface or retain a
safe retryable failure rather than silently forgetting ownership.

Add deterministic first-clear-fails/next-clear-succeeds tests for expiry and
explicit clear, plus copy/copy, old timer, external replacement, cancellation,
and stale retry schedules. The real `NSPasteboard` count+payload+clear remains
one main-thread closure; never auto-copy.

## Verification and handoff

Read every touched file in full after the last edit. Run focused onboarding,
storage, recovery, origin, export, and clipboard tests; full `swift test`;
high-count deterministic recovery/clipboard repetitions; release build;
strict formatting; `git diff --check`; scoped secret/URL/log scans; and
`task-board validate`. Tests must use injected stores/transports/file ops and
pasteboards and must not touch the real Keychain, system pasteboard, or user
files.

Write exactly one superseding outcome named
`TASK-260712-2u1w16_rework-r3-results.md`. Include the exact changed-file
inventory and SHA-256 hashes, R3-F1..F7 production/test mapping, exact commands
and results, dirty-tree boundaries, and honest unavailable gates. Set the task
to `to-review`; root full-file/hash/test audit and a fresh independent review
remain mandatory.
