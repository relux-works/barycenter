# TASK-260712-2u1w16 — independent security/migration review R2

Review only. Do not edit product/test files, commit, push, or mark the task
`done`. The producer handoff is untrusted evidence, not acceptance. Inspect
every producer-listed file in full, recompute every SHA-256, and compare the
implementation directly with the frozen Rev15 research, the implementation
guard, and root directives M1-M12.

The review must independently attempt to falsify, with concrete schedules and
production-seam tests where possible:

- canonical-origin construction and decoding, exact ICU UTS46 flags/errors,
  IPv4/IPv6 ambiguity, and endpoint/origin binding before any bearer send;
- bounded URLSession receipt, redirect/cancellation races, strict JSON depth,
  duplicate/unknown/trailing fields, endpoint status/code and role/slot/code
  semantics, and generic error/redaction behavior;
- distinct Keychain source/destination identities, DP/login duplicate and
  partial-update states, read-back-before-delete, cross-instance lost updates,
  structural validation, and legacy pair compatibility without plaintext
  fallback;
- recovery serialization/cancellation in both promotion orders, exact
  false-to-true send barrier across duplicate pending locations, no-secret
  restart, same-token retry, promotion-before-delete convergence, origin/actor/
  recovery/token identity, and node byte preservation;
- the impossible/corrupt-state boundary where an `ever_sent=false` pending
  record already matches the active bundle. Decide explicitly whether treating
  that as a successful prior promotion is justified by a separately confirmed
  credential, or whether it must fail closed; support the verdict from Rev15;
- direct export failure behavior and pasteboard copy/copy, copy/clear,
  cancellation/expiry, old-timer, and external replacement races. Verify the
  real NSPasteboard implementation performs compare-and-clear in one
  main-thread critical closure and never auto-copies.

Do not accept tests that merely mirror implementation, use timing as proof, or
omit the actual production seam. Run focused and full tests independently,
including repeated recovery/clipboard schedules and release build. Do not touch
the real Keychain or clipboard. Record unavailable runtime evidence honestly.

Write exactly one report named
`TASK-260712-2u1w16_independent-review-r2.md`, with verdict, severity-ranked
findings (file/line, failure schedule, violated invariant, required correction),
hash verification, exact commands/results, and remaining gates. Leave task
status `to-review` regardless of verdict; root alone accepts it.
