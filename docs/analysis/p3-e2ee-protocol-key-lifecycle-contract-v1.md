# P3 E2EE protocol and key-lifecycle audit contract v1

Task: `TASK-260712-2ys1ww`

Status: audit packet ready; production implementation disabled

Baseline: `73bdc18d14721936ad75c86cd135105175cc101e`

## Decision

The repository now has one bounded, candidate-neutral contract for independent review. It uses RFC 9420 lifecycle semantics, but it does **not** select an MLS library, cipher suite, platform binding, media container, or canonical MLS serialization. `e2ee_media_v1` is not advertised and no runtime path sends, stores, decrypts, or plays encrypted media.

The machine-readable authority is [`protocol/e2ee-media-audit-v1.json`](../../protocol/e2ee-media-audit-v1.json). Cross-platform audit vectors are in [`protocol/e2ee-media-audit-v1-vectors.json`](../../protocol/e2ee-media-audit-v1-vectors.json). The fixture suite is a label only; injected test verifiers exercise state transitions without implementing cryptography.

## Trust and visibility boundary

Clients own device private keys, MLS key-package private material, epoch secrets, media content keys, history grants, and recovery/transfer secrets. The coordinator may route only the metadata enumerated in the machine contract. Its strict decoder rejects plaintext, content keys, epoch/sender/recovery/history secrets, private keys, and any unknown field.

Authenticated data binds the exact contract/capability/suite, event, group, actor, verified device, Air, immutable target snapshot, media kind/object, epoch, generation, sequence, nonce, expiry, and manifest digest. Changing any bound value requires signature verification to fail in a future reviewed crypto adapter. The current model accepts only an injected verifier and has no production verifier.

## Lifecycle

1. A verified client device publishes a single-use key package. Another current verified device authors add/remove/update proposals and the commit.
2. A commit names the previous epoch and exact previous commit digest, advances exactly one epoch, and rotates target state. Concurrent commits against the same predecessor are serialized; a losing/stale branch is rejected and must rebase from an authenticated current state.
3. Offline clients restore epoch, commit, replay, generation, sequence, and nonce state before attempting decryption. They reject gaps/forks and resynchronize metadata before accepting new content.
4. Removal, leave, device revoke, recovery, or membership change requires an accepted epoch rotation before later content. Removing future access cannot erase plaintext or keys a recipient already copied.
5. A new/recovered device receives no history automatically. A current verified device may issue a recipient-device-, object/range-, epoch-, target-, and expiry-bound history grant. The coordinator routes only ciphertext.

## Media flows

- `clip`, `track`, and `saved_cue`: prepare locally, create one content key, encrypt through a future reviewed container, wrap that key for the current group/epoch, bind the immutable target snapshot and manifest, then route ciphertext. Reuse requires a fresh object generation/nonce domain as defined by the eventual reviewed suite.
- `live_ptt`: bind a session key to group epoch and session generation. Every sender frame has a monotonic sequence and unique nonce tuple. Reconnect/restart cannot reset the generation or replay window. Terminal/revoke/membership events rotate before later frames.
- report: the reporter explicitly decrypts and selects content locally, then exports the selected plaintext plus authenticated evidence. This is voluntary disclosure, not a coordinator decryption path.
- recovery/transfer: verify the new device independently, perform peer-approved secret transfer, rotate the epoch, then issue explicit history grants. A coordinator-only recovery path is forbidden.

## Fail-closed rules

All three models consume the same valid fixture and the same ten mutations. They reject unknown suite, invalid signature, tampered manifest, nonce reuse, stale epoch, forked epoch, replay, foreign target, expired grant, and downgrade. Production configuration has an empty suite set and rejects even the valid audit fixture as `unknown_suite`.

Unsupported or mixed-version clients must not see an encrypted affordance. Sending to an exact target snapshot that contains an unsupported client fails or requires the user to create a different explicit target snapshot; it never silently falls back to plaintext.

## Implementations under review

- Coordinator: `coordinator/internal/e2eecontract` is a keyless routing/state mirror with strict secret-field rejection and commit/content state tests.
- Windows: `pulsar-win/e2ee_contract_audit.go` is dormant and has no call site from `main`, WebSocket, storage, capture, or playback.
- macOS: `node-app/Sources/NodeCore/E2EEAuditContract.swift` is dormant and has no call site from registration, transport, storage, capture, or playback.

These are specification executables, not cryptographic implementations. Any future adapter must inject a reviewed verifier/library and keep the same failure taxonomy or version the contract.

## Independent audit handoff

The reviewer for `TASK-260712-aniuyy` must reproduce the Go, Windows, Swift, and acceptance tests from the exact hashes in `acceptance/phase3/e2ee-protocol-key-lifecycle-v1.json`. Review must confirm:

- no coordinator-visible secret or unbounded additive field;
- no capability advertisement, product claim, plaintext downgrade, or selected production suite;
- correct commit concurrency, stale/fork/replay, offline/restart, revoke, recovery, history, deletion-limit, and reporting rules;
- exact applicability to clips, streamed tracks, saved cues, and live PTT;
- all critical/high findings from the two no-go spikes remain explicit.

Production implementation stays blocked until that independent review is accepted and the later implementation tasks select reviewed dependencies and repeat platform/package evidence. Real signed-app and hardware testing remains in `EPIC-260714-th54l3`; no such evidence is claimed here.
