# Independent review brief — TASK-260712-tcwn44

Review exact rework commit `8c2676206f3fdb44ed54b9ad6f3dc1c5af5728af` for
`TASK-260712-tcwn44 — macos-protected-media-playback`.

Act as an independent security, correctness, lifecycle and realtime-audio
reviewer. Do not modify the worktree. Inspect the complete branch through the
exact commit and the rework diff from `b509f85034a91fcbcc756d236fe05979085825d9`.
Read the prior REJECTED verdict and verify that M1 and both Low findings are
actually closed without introducing a new Critical/High/Medium issue. Verify
the task README acceptance criteria and checklist, and
report every Critical, High, Medium, Low and informational finding with exact
file/line evidence and a concrete failure scenario. Re-run relevant tests.

Pay special attention to:

- manifest/envelope/sender/group/recipient/epoch/generation/target binding;
- ciphertext hash and provider authentication ordering before decoder access;
- dynamic expiry, group revision and history-grant revocation between prepare,
  fetch, decrypt, seek and restart;
- ciphertext-only durable cache, tombstone semantics, plaintext/key lifetime,
  cancellation and concurrent state changes;
- canonical range binding, cache substitution, replay and downgrade behavior;
- optional protected reader injection into `MacStreamCandidatePlayer` without
  regression of generation/seek/deadline/PCM-ring/receipt semantics;
- production-dark claims, absence of runtime/capability wiring, and honesty of
  fixture/manual evidence;
- hash-pinned acceptance packet and regression cascade.
- concurrent cache actors on one root: stale hits, tombstone monotonicity,
  entry merge/removal, same/different variant writes, corrupt/duplicate index
  input, temporary-file collisions, limits and restart behavior;
- legitimate history re-grant after membership rotation and player lifetime
  after the caller releases its prepared wrapper.

Required commands include:

```sh
git show --stat --oneline 8c2676206f3fdb44ed54b9ad6f3dc1c5af5728af
git diff b509f85034a91fcbcc756d236fe05979085825d9 8c2676206f3fdb44ed54b9ad6f3dc1c5af5728af
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacProtectedMediaPlaybackTests
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacStreamTrackPlayerTests
python3 -m unittest scripts.acceptance.test_macos_protected_media_playback scripts.acceptance.test_stream_performance_review
```

Use the task-board verdict format. The final verdict must be exactly ACCEPTED or
REJECTED. ACCEPTED requires no open Critical, High or Medium finding. Explicitly
separate production-blocking deferred external/manual gates from defects in this
producer commit. Save the verdict as an outcome resource for this task.
