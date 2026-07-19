# Independent delta review: TASK-260712-1x9ruo (macOS E2EE key state)

Reviewer: implementation-independent (Claude, reviewer role). I did not author or modify any reviewed production code, test, vector, validator, or evidence artifact; my access was read-only and the only writes were board resources/notes/status via the task-board CLI.

## 1. Exact target and artifact verification

- `git rev-parse HEAD` = `498957eab686a4e6aad0f653813ccfe3d1d3efa6` (exact producer SHA; baseline merge `3b08b745…33a`).
- `git diff HEAD --stat -- ':!.task-board'` is empty: every reviewed file is byte-identical to the producer commit. Only board progress files were dirty.
- All 9 artifact SHA-256 hashes in `acceptance/phase3/macos-e2ee-key-state-v1.json` independently reproduced with a standalone Python script: repository, repository-tests, state-vectors, adr, threat-model, key-lifecycle, schema-foundation, routing-rotation, opaque-router — all OK.
- No `node-app/Package.swift` change in the delta (no dependency/library introduced).

## 2. Independent commands, counts, timings

| Command | Result | Timing |
|---|---|---|
| `xcrun swift-format lint` (source + tests) | clean, no diagnostics | 0.23 s |
| `xcrun swift test --package-path node-app --filter MacE2EEKeyStateTests` | 10/10 tests, 1 suite, pass | 3.5 s wall |
| `xcrun swift test --package-path node-app` | 318 tests / 53 suites, pass | 2.5 s wall (warm) |
| `python3 -m unittest scripts.acceptance.test_macos_e2ee_key_state` | 5 tests OK (0.005 s) | 0.09 s wall |
| `python3 scripts/acceptance/validate_macos_e2ee_key_state.py` | `macOS E2EE key state: PASS (production disabled)` | 0.05 s |
| `python3 scripts/acceptance/run_automated.py` (full battery) | 16/16 commands exit 0, manifest `status: pass` at HEAD `498957e`; contract-test step: **Ran 217 tests … OK** (89.98 s) | 4 m 16 s total |

Toolchain: Xcode 26.2 (17C52), Swift 6.2.3, go 1.25.12, Darwin 24.6.0. No failures anywhere.

Independent source scans (not taken from the packet):
- `grep MacE2EEKeyState|e2ee_media_v1` across `node-app/Sources/NodeApp/` (incl. `main.swift` and all `Mac*AppComposition.swift`): zero hits — repository is unreferenced by any composition root.
- `grep UserDefaults|os_log|Logger(|print(` in `MacE2EEKeyState.swift`: zero hits.
- `MacE2EEKeyStateRepository`/`SystemMacE2EEKeychainStore` referenced by no production source other than its own file.

## 3. Assessment of required review questions

**Q1 Keychain isolation — PASS.** Six distinct state slots (`device_metadata`, `device_signing`, `device_agreement`, `group/<id>`, `grant/<id>`, `content-cache`), each with an independent witness item, under accounts `state.<kind>.<sha256(scope)>` / `witness.<kind>.<sha256(scope)>` in the dedicated service `works.relux.pulsar.e2ee` (`MacE2EEKeyState.swift:34`, `:856-861`). Every query sets `kSecUseDataProtectionKeychain: true` and `kSecAttrSynchronizable: false` (`:38-46`); `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` is applied on add (`:64`) and persists through `SecItemUpdate` (only `kSecValueData` is replaced). Signing and agreement keys never share a Keychain value.

**Q2 Persist-before-ack — PASS.** `persist()` (`:784-822`) order: revision CAS → canonical record write → exact-byte record readback → witness write (revision + record digest) → exact-byte witness readback → full pair reload and equality with the in-memory record → only then return. The validator independently pins this ordering by source index. Crash window A (record written, witness not): revision/digest mismatch in `loadRecord` (`:830-845`) → `rollbackOrClone`; covered by `crashAfterRecordBeforeWitnessFailsClosedWithoutGenerationReuse`. Crash window B (both written, final readback lost): mutation stays consumed — restart observes generation 1 / revision 2 and a retry with the old revision gets `conflict`; covered by `lostReadbackAfterBothWritesConsumesGenerationWithoutReuse`. An ambiguous success can therefore skip but never reuse a generation. Generation overflow guarded at `UInt64.max` (`:457` → `replay`).

**Q3 Epoch/replay/fork/clone — PASS with honestly disclosed limits.** Exact predecessor digest + exactly-one-epoch advance enforced (`:388-391`); stale epoch → `staleEpoch`; epoch gap and wrong predecessor → `rollbackOrClone`; stale revision → `conflict`; cross-installation copied group state rejected via installation binding in record+witness (`:838-839`, test `copiedGroupStateCannotCrossInstallationWitness`); partial 3-slot device install fails closed on load and on retry; deletion removes the record before the witness so a deletion crash cannot resurrect a usable secret (`:848-854`); malformed/non-canonical payloads fail the canonical round-trip (`:871-879`) → `corrupt`; expiry and revocation behave per AC. I verified there is no epoch-arithmetic trap: `epoch <= payload.epoch` (`:388`) dominates before `payload.epoch + 1` (`:390`), so the `+1` cannot overflow. What the unkeyed witness and process-local lock actually provide: detection of crash-torn writes, partial installs, and cross-installation copies, and in-process serialization. They do **not** defend against a writer that can rewrite both items coherently, a coherent rollback of a full record+witness pair, or a whole-device clone of all slots — the ADR (`docs/analysis/p3-macos-e2ee-key-state-v1.md`, "Persist-before-ack protocol") disclaims exactly this; no overclaim found in ADR, packet, or vectors.

**Q4 Secret boundary — PASS.** One fixed, generic `errorDescription` for every failure case (`:17-19`), no interpolation of identifiers or key material into any error or description; all leases print `<redacted>` in `description`/`debugDescription`; secrets are exposed only through closure-scoped `withUnsafeBytes`-style accessors; `destroy()` zeroes the backing buffer best-effort with `deinit` backstop, and the ADR correctly limits the claim (Swift/Foundation copies preclude guaranteed wiping). Independent grep confirms no preferences, logging, telemetry, or crash-diagnostic writes. Bounds enforced and validated on both write and read: 4 KiB private keys, 1 MiB opaque group state, 64 KiB grants, 32 entries / 64 KiB content-key cache with expiry eviction and full clear.

**Q5 Production-dark boundary — PASS.** No group-crypto library, cipher suite, protected container, or key-generation algorithm introduced (no Package.swift delta; keys are caller-supplied opaque blobs). CryptoKit SHA-256 is used only for canonical record digests and account-name tokens and the ADR explicitly frames it as consistency witnessing, not production crypto. No composition-root wiring, no `e2ee_media_v1` advertisement (independent grep), no plaintext fallback path, and the packet decision block is all-false with `deltaReviewRequired: true`.

**Q6 Contract alignment — PASS.** EPC-005 target semantics implemented and vector-pinned: active member with only revoked/unverified devices → `removed_endpoint` (never silently reclassified), verified-but-unsupported → `unsupported_target`, verified+supported → `route`; malformed/inconsistent inputs fail closed to `removed_endpoint` (`MacE2EETargetDevicePolicy`, `protocol/e2ee-key-state-v1-vectors.json`, validator pin). Threat-model, key-lifecycle, schema, routing-rotation, and opaque-router upstream packets are hash-pinned and reproduce. Deferred manual epic `EPIC-260714-th54l3` recorded; open gates `EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, `TASK-260712-1ulshp` all present and pinned by the validator.

**Q7 Evidence integrity — PASS.** All hashes reproduce (section 1). The validator is not circular: it pins artifact hashes, verifies persist-before-ack ordering by source index, scans for forbidden tokens, independently reads the production composition root, forces every `manualEvidence` value to `not-run`/`not-run-no-selected-stack`, and requires the exact open-gate set; the unittest wrapper proves it fails closed on production enablement, Keychain-policy drift, bound drift, and invented manual evidence. No manual, signed-build, hardware, backup, or real-crypto evidence was invented anywhere.

## 4. Findings

**Critical — none.**

**High — none.**

**Medium:**
- **M1 (concurrency scope): serialization is process-local only; there is no cross-process CAS at the Keychain layer.** `processLock` (`MacE2EEKeyState.swift:265`) serializes mutations within one process, but `loadRecord`→`persist` is not atomic across processes: two processes in the same Keychain access group could both read revision N and both write revision N+1, allowing duplicate send-generation reservations (nonce-reuse hazard in a live system). *Disposition: outside this dormant scope with a tracked owner.* The ADR states serialization is "inside the owning process"; the repository is unwired, production-dark, and `node-app` has a single executable target today. The deferred runtime-integration scope (`deferredScope: send-playback-live-ptt-and-ux-runtime-integration`, plus the open production gates) must either enforce single-instance ownership of this store or add cross-process serialization before any runtime wiring. This must be an explicit acceptance criterion of those integration tasks.

**Low:**
- **L1:** A crash between the three device-slot persists during first-time install (`:319-330`) leaves identity slots permanently failing closed (`rollbackOrClone`) with no public reset/recovery interface. Correctly fail-closed and explicitly deferred to the recovery/re-verification flow in the ADR; noting so the recovery task inherits it.
- **L2:** Expired grants are never garbage-collected and grant slot count is unbounded (one record+witness pair per `grantID`, each ≤64 KiB). Expiry is enforced on load and revocation works; integration should add lifecycle cleanup.
- **L3:** The canonical round-trip check (`canonicalDecode`, `:871-879`) depends on Foundation `JSONEncoder` byte-stable output (`sortedKeys`, `withoutEscapingSlashes`) across OS/Swift updates; an encoder formatting change would render stored state `corrupt`. This fails closed to state loss + re-verification, consistent with the threat model, but is a real operational sensitivity to track.

**Informational:**
- **I1:** Unkeyed witness limits (coherent pair rollback / access-group-capable attacker undetectable) are accurately disclosed in the ADR — recorded here to prevent future evidence from citing the witness as tamper-proofing.
- **I2:** The idempotent re-install path compares caller-supplied private keys to stored keys with non-constant-time `Data` equality (`:305`). Local API, negligible practical risk.
- **I3:** `validIdentifier` permits `/` and `.` in device/group/grant IDs; no collision is possible because full scope strings are SHA-256-hashed into account names and kinds are segregated, but a tighter charset would be simpler to reason about.
- **I4:** `requireInstallation` performs six Keychain reads per operation; fine for dormant scope, worth caching consideration at integration time.

## 5. Verdict

**APPROVE WITH NON-BLOCKING FOLLOW-UPS** (M1 dispositioned to the deferred runtime-integration/recovery scope with the tracked owners named above; L1–L3 inherited by the same deferred tasks).

## 6. Production limitations and still-open gates

This commit is a production-dark, device-only engineering foundation. It does not select or prove any group-crypto library, cipher suite, or protected container; performs no real key generation; is not wired into any runtime or capability advertisement; and has no signed-build, physical-hardware Keychain, backup/restore, memory-forensics, or crash-diagnostic evidence (all `not-run`, deferred to manual epic `EPIC-260714-th54l3`). Production enablement remains blocked by `EPC-001`, `EPC-002`, `EPC-004`, `EPC-005`, and external security closure `TASK-260712-1ulshp`. Single-process ownership (M1) must be resolved before runtime wiring.
