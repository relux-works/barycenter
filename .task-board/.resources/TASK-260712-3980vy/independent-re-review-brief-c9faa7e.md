# Independent re-review brief — TASK-260712-3980vy

Re-review exact rework commit `c9faa7e` for
`TASK-260712-3980vy — macos-e2ee-live-ptt` against rejected producer commit
`d8a429c` and outcome `TASK-260712-3980vy_review-verdict-v1.md`.

Act as the independent security/protocol/lifecycle/concurrency/realtime
reviewer. Do not modify production code. Verify that HIGH-1 and both Low
findings are actually closed without a new Critical/High/Medium regression.
Inspect the exact rework diff, full current files, updated ADR/vectors/packet,
hash pins and accepted E2EE design authority.

Required focus:

- HIGH-1: cross-device AAD must never contain the device-local Keychain record
  revision. Confirm local revision is used only for setup/CAS, while both peers
  bind the same witnessed epoch + commit digest. Inspect and run the new
  two-installation fixture: sender and receiver use separate repositories and
  local revisions deliberately differ after sender generation reservation,
  yet protect→open succeeds with the same shared commit.
- Treat the AAD binding change as a reviewed cross-client contract delta even
  though the public `BE` wire is byte-identical. Confirm the updated ADR,
  vectors, acceptance invariant and source all agree on `commit_digest` and no
  stale `group_revision` remains in AAD.
- LOW-1: malformed/empty/oversized provider output must be classified
  separately from nonce reuse and terminate fail closed.
- LOW-2: sequence 15001 must fail before `crypto.seal`, without consuming a
  nonce or growing state; inspect the 15,000-bound fixture and seal count.
- Reconfirm byte-exact Go/Swift `BE` parity, retry idempotence, auth-before-
  jitter, membership teardown, callback isolation, production darkness,
  ownership gate, bounds and no invented hardware evidence.

Producer evidence at exact `c9faa7e`:

```sh
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift-format lint --strict node-app/Sources/NodeCore/MacE2EELivePTT.swift node-app/Sources/NodeCore/MacE2EEKeyState.swift node-app/Tests/NodeCoreTests/MacE2EELivePTTTests.swift
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacE2EELivePTTTests
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app
python3 -m unittest discover -s scripts/acceptance
python3 scripts/acceptance/run_automated.py
```

Producer reports formatter clean, focused 10/10, full Swift 350/350,
acceptance 200/200 and harness 16/16. Reproduce the relevant evidence.

Use task-board verdict format. Terminal verdict must be exactly `ACCEPTED` or
`REJECTED`; acceptance requires zero open Critical/High/Medium. Save a new
outcome resource (do not overwrite v1). Set task `done` only on acceptance;
otherwise return it to `to-dev`. Manual hardware/real-provider work remains in
`EPIC-260714-th54l3` and is not a coding defect. Do not claim production E2EE.
