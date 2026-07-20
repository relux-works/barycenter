# Independent review brief — TASK-260712-3980vy

Review exact producer commit `d8a429c44a39971d89a739c7a0ab610fce16d9b5` for
`TASK-260712-3980vy — macos-e2ee-live-ptt`.

Act as an independent security, protocol, lifecycle, concurrency and realtime-
audio reviewer. Do not modify production code or the producer worktree. Inspect
the complete commit and diff from its parent. Verify the task README, all six
checklist items, the accepted design authority, the existing coordinator `BE`
opaque-live contract, the acceptance packet and every pinned artifact. Report
every Critical, High, Medium, Low and informational finding with exact file/line
evidence and a concrete failure scenario.

Pay special attention to:

- byte-for-byte Swift parity with the existing Go `BE` wire without a protocol
  delta or legacy `BP` downgrade;
- witnessed identity/group state, the dedicated crash-safe `live_ptt`
  generation reservation, exact epoch/revision/target re-read and session-key
  provider derivation boundary;
- canonical AAD completeness and ambiguity resistance across group, Air,
  sender device/actor/orbit/node, target, session, epoch, group revision,
  generation, sequence, flags, codec and timing;
- unique nonce enforcement in both directions, retry idempotence without
  resealing, replay/gap/out-of-order behavior and sequence/timestamp bounds;
- authentication before any jitter/Opus/FEC/PLC access, fail-closed tamper,
  stale epoch, foreign target, removed sender and verified membership changes;
- crypto and Keychain/network/file work staying off microphone capture and
  audio render callbacks; preservation of existing DND, backpressure, jitter,
  FEC/PLC, PCM and teardown ownership;
- secret/session destruction exactly once, terminal-state races, lock scope,
  Sendable claims, memory growth and malformed provider output;
- one-owner and cross-process generation semantics. Distinguish a real bypass
  in the dormant code from the explicitly forbidden runtime composition while
  external serialization approval remains absent;
- production-dark honesty: no selected suite/provider/container, no NodeApp
  composition, no capability/UI claim, no plaintext fallback/persistence, and
  no invented real-app/hardware/packet-capture evidence;
- hash cascade correctness and whether this is truly non-protocol-affecting.

Producer evidence at exact commit:

```sh
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift-format lint --strict node-app/Sources/NodeCore/MacE2EELivePTT.swift node-app/Sources/NodeCore/MacE2EEKeyState.swift node-app/Tests/NodeCoreTests/MacE2EELivePTTTests.swift
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacE2EELivePTTTests
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app
python3 -m unittest discover -s scripts/acceptance
python3 scripts/acceptance/run_automated.py
```

The producer reports 8/8 focused Swift, 348/348 full Swift, 200/200 acceptance
discovery, 16/16 automated harness and strict swift-format clean. Reproduce the
relevant battery independently. Manual real-app/hardware work is owned by
`TASK-260712-flaiie` and `TASK-260712-yj668d` in `EPIC-260714-th54l3` and is
not a requirement for accepting this production-dark coding task.

Use the task-board verdict format. The terminal verdict must be exactly
`ACCEPTED` or `REJECTED`. `ACCEPTED` requires zero open Critical, High or
Medium finding. Save the verdict as an outcome resource for this task and set
the task to `done` only on acceptance; otherwise return it to `to-dev` with the
required fixes. Do not claim production E2EE readiness.
