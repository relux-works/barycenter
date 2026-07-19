# Independent delta review: TASK-260712-1x9ruo

Role: implementation-independent reviewer. Do not edit production code, tests, evidence, planning, or board status manually. Review the exact producer commit and attach a written outcome resource with an explicit verdict.

## Exact review target

- Repository: `/Users/administrator/Developer/Ivan/barycenter`
- Branch: `feat/task-260712-1x9ruo`
- Exact producer SHA: `498957eab686a4e6aad0f653813ccfe3d1d3efa6`
- Baseline merge: `3b08b745590d36e17c6daf8ffe7ef8007decc33a`
- Scope: production-dark macOS E2EE key-state foundation only.

Begin by proving `git rev-parse HEAD` is the exact producer SHA and that all reviewed files are byte-identical to that commit. If the workspace moved, use read-only `git show`/a detached temporary worktree; do not review an uncommitted delta and do not mutate the producer branch.

## Required review questions

1. Keychain isolation: confirm device metadata, signing key, agreement key, group state, grants, and bounded content-key cache use distinct state slots and independent witnesses under the dedicated service. Confirm the production store uses the data-protection Keychain, `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`, and explicit non-synchronizing behavior.
2. Persist-before-ack: trace exact record write/readback, witness write/readback, final pair validation, revision checks, and the ordering of returned send reservations. Challenge both crash windows: state written without witness; both written but final readback lost. Confirm an ambiguous success can skip but cannot reuse a generation.
3. Epoch/replay/fork/clone: challenge exact predecessor digest, one-epoch advance, stale revision, epoch gap, cross-installation copied group state, partial identity installation, deletion, overflow, malformed payload, expiry, and revocation behavior. Call out precisely what the unkeyed witness and process-local lock do and do not protect; reject any overclaim.
4. Secret boundary: inspect redacted descriptions/errors, closure-only leases, best-effort clearing claims, absence of preferences/log/telemetry/crash writes, cache/grant/state bounds, and failure behavior. Look for accidental secret copies, log interpolation, or runtime exposure.
5. Production-dark boundary: confirm no group-crypto library, cipher suite, protected container, key generation algorithm, runtime app composition, production capability advertisement, or plaintext fallback was introduced. CryptoKit SHA-256 must be only local canonical integrity/account hashing, not presented as production crypto evidence.
6. Contract alignment: verify EPC-005 target semantics (only revoked/unverified endpoints => `removed_endpoint`; verified unsupported => `unsupported_target`), threat/lifecycle/schema/routing/opaque upstream pins, deferred manual epic `EPIC-260714-th54l3`, and all open EPC/external gates.
7. Evidence integrity: reproduce all hashes in `acceptance/phase3/macos-e2ee-key-state-v1.json`; inspect the validator for circular or superficial assertions; verify no manual/signed/hardware/backup/real-crypto evidence was invented.

Pay special attention to single-process ownership versus cross-process CAS, coherent rollback limits of an unkeyed record+witness pair, partial multi-slot device install recovery, Keychain query attributes on update/read/delete, canonical decoding, integer overflow, cache byte accounting, and whether the public interfaces are narrow enough for downstream send/playback/live/UX work.

## Independent commands

At minimum, independently run and record exact results/timings for:

```sh
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift-format lint node-app/Sources/NodeCore/MacE2EEKeyState.swift node-app/Tests/NodeCoreTests/MacE2EEKeyStateTests.swift
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter MacE2EEKeyStateTests
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts.acceptance.test_macos_e2ee_key_state
PYTHONDONTWRITEBYTECODE=1 python3 scripts/acceptance/validate_macos_e2ee_key_state.py
```

Also run the exact contract acceptance list from `scripts/acceptance/run_automated.py` if time permits; producer reports 217/217. Source-search production composition roots and ordinary diagnostics independently rather than accepting the packet statement.

## Verdict format

Attach an outcome resource named `TASK-260712-1x9ruo_independent-delta-review-v1.md` containing:

- exact SHA and artifact/hash verification;
- commands, counts, timings, and failures if any;
- findings grouped Critical/High/Medium/Low/Informational with file/line evidence;
- explicit assessment of each required review question;
- explicit statement that no reviewed code was authored or modified by the reviewer;
- one terminal verdict: `APPROVE`, `APPROVE WITH NON-BLOCKING FOLLOW-UPS`, or `REJECT`;
- explicit production limitations and still-open gates.

Any open Critical or High finding rejects. A Medium finding must be dispositioned explicitly and normally blocks acceptance unless it is demonstrably outside this dormant engineering scope with a tracked owner. Manual real-app/hardware evidence cannot be accepted by this review.
