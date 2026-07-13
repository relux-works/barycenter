# TASK-260712-2u1w16 — independent security/migration review R4

Date: 2026-07-13 (Asia/Tbilisi)

This is a review-only run. The R3 producer handoff is not accepted. Independently
attempt to falsify the implementation and its evidence. Do not edit production
or test sources, do not commit, push, clean, reset, or touch user Keychain,
pasteboard, selected files, or live coordinator state. Review-only adversarial
tests may be created under a fresh `/tmp` copy. The only workspace output is the
required report resource.

Read in full:

- `TASK-260712-2u1w16_rework-r3-results.md` (SHA-256
  `293ba3fc4d74ea4521fdbc7d2822b5121eb5c25cf918255932885201be11e32c`)
- `TASK-260712-2u1w16-rework-guard-r3.md` (SHA-256
  `142f148b9cf7ef177c1ff4f9710ef2e8a955234b36fe04eca7b7648e1a55af7f`)
- the prior independent report `TASK-260712-2u1w16_independent-review-r2.md`
  (SHA-256 `41478cc49106c11975aa39bd33954d24b683660c11015deeb8813f1ca55011d8`)
- every changed production file and every changed test file listed below, in
  full; inspect relevant frozen dependencies rather than trusting producer
  summaries.

## Frozen R3 boundary

Abort and report a boundary violation if any hash differs before or after the
review:

```text
5574ecf4377a19a37eeaa31f9ea7ca33e6eb1e9c094313858accf2198581b0cf  node-app/Sources/NodeCore/Keychain.swift
32e4009a3cf9177d23cb88fea65601f4642f2ffe0e20b51e9b587bd48440964a  node-app/Sources/NodeCore/CoordinatorOrigin.swift
c6abefdf7cfc5b0694c7aebfbe28e6de88762079d67f948246f875d2e750272f  node-app/Sources/NodeCore/OnboardingCredentials.swift
9d2429ec2dbc04ff5b2a5c934b7f87b27d969e96ba7befb5831769046e1e721a  node-app/Sources/NodeCore/OnboardingHTTPClient.swift
94135aeaaac8b1b728253498ab61e2a5220f72a5e62d54be1381a972279cd1b7  node-app/Sources/NodeCore/RecoveryService.swift
bdcaf2f478aee55eed98eade76be2f5e7220d67737094481c3366a562fc215e8  node-app/Sources/NodeCore/RecoveryExport.swift
43ec50ccec2396647cc976800adb90d7001490538dc637558aac06c434bf4c44  node-app/Tests/NodeCoreTests/CoordinatorOriginTests.swift
afa8ab8e61ac6580823a78d676891a9f10b5fea4c72e6f12181b7b62a856f779  node-app/Tests/NodeCoreTests/CredentialBundleTests.swift
6676f67cd01ef721b487dc9afd5cdc4c0b19294b2a536e04de4007323a634dbd  node-app/Tests/NodeCoreTests/OnboardingTestSupport.swift
fe914849b75e3579d27b293da10cedf0d9fd880e7fe400ac3034d159c7d47cc1  node-app/Tests/NodeCoreTests/RecoveryExportTests.swift
02b02d374ccd076823a0823a6a893d8c98000338fc5d9eef5c03f2f4f6cb4d28  node-app/Tests/NodeCoreTests/RecoveryServiceTests.swift
9779a339c3dcca86f5a7b0f62bbc6f90befd6cba88fad071875fdb49b72c5a80  node-app/Sources/NodeApp/main.swift
```

## Mandatory falsification areas

1. Reproduce each R2/R3 finding and prove its repair against production seams,
   not only test doubles: unsent-pending/active conflicts; strict byte/schema
   equivalence; durable active-vs-limited classification; UTS46 trailing-root
   ordering; checked export close; value-independent descriptions; clipboard
   retry/lease ownership.
2. Audit every crash boundary of protected-store migration and recovery:
   destination add/update/readback, source delete, active save, pending delete,
   duplicate DP/login states, sent/unsent generation conflicts, pair-vs-recovery
   concurrency, and restart with zero unauthorized network sends.
3. Audit secret lifetime and disclosure: Codable/reflection/descriptions/errors,
   URL/request/log construction, export partial-file cleanup, clipboard
   compare-and-clear replacement races and retry exhaustion. Search both source
   and release binary using distinctive canaries, while distinguishing intended
   test constants from production disclosure.
4. Verify canonical-origin and capability binding under Unicode dot mappings,
   IDNA edge cases, ports, userinfo, paths, query/fragment, literal loopback,
   node/control origin mismatch, orbit mismatch, and limited credentials that
   retain cached metadata without becoming active.
5. Examine strict decoders for duplicate keys, unknown keys, `null` in required
   or optional scalar fields, numeric edge cases, noncanonical encodings,
   trailing data, and byte-different decoded lookalikes.
6. Audit direct export at the actual Darwin seam: exclusive/no-follow open,
   EINTR and short/zero writes, fsync, exactly one checked close, best-effort
   cleanup after every failure, no success before close, no sidecar/temp leak.
7. Audit clipboard retry liveness: expiry and explicit-clear transient errors,
   exact lease retention, replacement detection, stale timers, copy/copy,
   bounded retry delay/count, terminal error visibility, and no clearing of
   externally replaced content.
8. Independently rerun focused tests, the full suite, release build, formatting,
   diff check, and any deterministic stress schedules needed. Do not exercise
   real user data. Record exact commands, counts, and results.

Do not weaken the contract merely to make tests pass. Severity-rank every
finding with exact file/line references, a concrete exploit/crash schedule,
and the missing test. If no defect is found, explicitly state which adversarial
schedule falsifications were attempted and why they are sufficient.

Write the final review to
`TASK-260712-2u1w16_independent-review-r4.md`, attach it as an outcome resource,
and include final hashes for all frozen files. Verdict must be exactly one of:
`ACCEPT FOR ROOT AUDIT` or `BACK TO DEVELOPMENT`. Do not mark the task done;
root retains acceptance authority.
