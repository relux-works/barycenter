# Independent review brief — TASK-260712-2kcduo

Review the exact producer commit
`30d23def4350aab22a19824c1e0cbcfad1a5f8da` in a detached worktree. Do not
review a moving branch and do not modify production artifacts.

This is an implementation-independent review of the production-dark macOS
protected-media send foundation and the protocol/design delta it introduces.
Return a terminal `ACCEPTED` or `REJECTED` verdict with Critical/High/Medium/
Low/Info findings, exact file/line evidence, reproduced commands and explicit
non-claims. Publish the verdict as an outcome resource named
`TASK-260712-2kcduo_review-verdict.md`.

Review priorities:

1. Verify the task acceptance criteria and all six checklist items against the
   exact code, tests, vector, ADR and acceptance packet. The no-selected-stack
   constraint is binding: no real codec/container/crypto claim may be inferred
   from the deterministic audit fixture.
2. Confirm `MacProtectedMediaSendService` remains absent from the NodeApp
   composition root, the public initializer cannot run an unapproved provider,
   `e2ee_media_v1` is unadvertised, and no plaintext/downgrade route reaches the
   coordinator or legacy media pipeline.
3. Audit rights/target admission, exact recipient binding, stale revision and
   epoch behavior, generation reservation ordering, nonce uniqueness, provider
   authentication, artifact bounds, source and ciphertext integrity,
   idempotency keys, chunk offsets, retry after every ambiguous boundary, and
   finalize semantics. Look specifically for nonce/object reuse after crash.
4. Audit file ownership and symlink handling, source lifecycle, user-owned
   retention, app-owned deletion, explicit cancel, expiry recovery, permissions,
   path traversal, tamper and cleanup failure behavior. Check that no plaintext,
   secret, path or key is logged or persisted outside the declared policy.
5. Verify the non-releasable single-send-owner claim resolves the earlier
   macOS key-state review's process-local duplicate-owner concern for this
   dormant one-process integration. State clearly that it is not cross-process
   serialization and remains a gate if packaging later adds another process.
6. Recompute every hash in
   `acceptance/phase3/macos-protected-media-send-v1.json`; verify the cascade
   updates to the macOS key-state, Windows key-state and recovery packets; run
   the fail-closed acceptance mutation tests.
7. Decide whether the implementation is compatible with the approved
   candidate-neutral design or whether any protocol-affecting delta reopens a
   Critical/High design gate. Do not silently bless a suite/container/provider.

Required fresh evidence at minimum:

```text
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacProtectedMediaSendTests
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacE2EEKeyStateTests
python3 -m unittest scripts.acceptance.test_macos_protected_media_send
python3 -m unittest discover -s scripts/acceptance -p 'test_*.py'
```

Run broader repository suites when needed to support the verdict. Treat all
signed/notarized app, physical Keychain, real crypto interop, real codec,
hardware, memory/crash/swap/backup and audible-media claims as `not-run` and
owned by `EPIC-260714-th54l3` or the named open production gates.
