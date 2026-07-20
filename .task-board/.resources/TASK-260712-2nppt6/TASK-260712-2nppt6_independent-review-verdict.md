# Independent review verdict — TASK-260712-2nppt6

- **Task**: TASK-260712-2nppt6 — Implement macOS E2EE verification recovery grant and report UX (macos-encrypted-media-client-path)
- **Producer commit reviewed**: `3a64b1808ce990fbef2cfb737839a15cbd0f6cbb` (exact)
- **Baseline parent**: `fae94974da0dfeaa4284820070d806dd7c986b0a`
- **Reviewer**: independent Claude Fable 5 exact-SHA reviewer (read-only; no product code modified)
- **Date**: 2026-07-20

## Terminal verdict

**ACCEPTED** — zero open Critical, High, or Medium findings.

## Diff and cleanliness gates

- `git rev-parse HEAD` = `3a64b18…` during the entire review; the working tree
  differed from the reviewed commit only by board tracking files
  (`.task-board/*progress.md` status transitions and the uncommitted reviewer
  brief), which are outside the producer diff.
- The commit touches exactly 17 files: the SwiftUI model/view, the dormant
  NodeApp composition, model tests, portable protocol contract, acceptance
  evidence record, acceptance validator/tests, harness registration, analysis
  doc, planning update, and board tracking/resource copies. Board resource
  copies of the contract, evidence, and analysis docs are byte-identical to the
  repo files (verified with `diff`).

## Audit findings by required focus item

1. **Production-dark confirmed.** No file in `node-app/Sources` outside the
   three new files references `PulsarEncryptedMediaView`,
   `PulsarEncryptedMediaModel`, or `MacEncryptedMediaClientPathComposition`
   (grep-verified); `main.swift` contains neither symbol (also enforced by the
   Swift source-contract test and the Python validator).
   `MacEncryptedMediaGenerationOwnershipLease` has no concrete conformance
   anywhere in the tree, so the composition cannot be constructed by
   configuration or flag. `PulsarEncryptedMediaView` is `internal` to
   `NodeAppUI`, so the executable target cannot embed it without a deliberate
   future visibility change. The portable contract pins
   `runtime_wired=false`, `capability_advertised=false`,
   `selected_production_suite=null`, and the validator fails closed on each
   (mutation-tested). No claim of signed/notarized hardware behavior is made;
   those claims are explicitly deferred to `EPIC-260714-th54l3` in the
   evidence record (`manualDeferred`, 7 items) and analysis doc.
2. **Single repository, single lease, no double reservation.** The composition
   creates exactly one `MacE2EEKeyStateRepository` and passes it to
   `MacProtectedMediaSendService`, `MacProtectedMediaPlaybackService`, and
   `MacE2EELiveSessionFactory` (`MacEncryptedMediaClientPathComposition.swift:47-64`).
   The lease is retained for the composition lifetime
   (`private let ownershipLease`) and construction throws
   `ownershipUnattested` unless the lease attests cross-process coverage with
   a non-empty scope. Repository-side generation ownership is claim-exact:
   `claimProtectedMediaSendOwnership()` (send) and
   `claimE2EELiveSendOwnership()` (live) are distinct one-shot, non-releasable
   claims under `processLock` (`MacE2EEKeyState.swift:290-306`), so one send
   service plus one live factory over one repository cannot double-reserve,
   and a second service instance would throw `conflict`. Playback takes no
   generation claim (read path). Source-contract tests and the validator pin
   the one-repository composition seams.
3. **Runtime-sensitive commands fail closed.** The presentation model reports
   an encrypted path only when every gate holds: surface `ready`, no command
   in flight, `runtimeWiringApproved`, `reviewedSuiteSelected`,
   `capabilityAdvertised`, `keyStateReady`, `sameRepositoryWitness`,
   safe ownership, `verified` this-device, `current` membership, nonzero
   epoch, and (for send paths) explicit unsupported-recipient exclusion.
   `replace()` normalization makes false-encrypted state unrepresentable:
   unsafe ownership or a broken repository witness forcibly clears
   `runtimeWiringApproved` and `capabilityAdvertised`, and a missing
   suite/wiring clears `capabilityAdvertised`. A blocked protected path stays
   selected and visibly blocked (`availability == .blocked`); the model never
   rewrites the selection to plaintext, and `selectPathCommand` refuses
   protected paths that are not fully `encrypted`. Plaintext can only be
   chosen by an explicit user selection command. Verified by
   `protectedStatusFailsClosed` and `unsupportedTargetsNeverDowngrade`.
4. **Contract-conformant flows.** Verify and revoke are separate,
   capability-gated commands; revoke requires explicit confirmation, warns
   that pending transfer/grant state is revoked, and the rotation-required
   banner blocks protected send until a new epoch
   (`deviceLifecycleCommandsFailClosed` covers verified/unverified/this-device
   and rotation cases). Device transfer requires a verified, current,
   non-this-device target and full cryptographic readiness, and copies the
   current epoch only ("History is not included" confirmation). User-held
   recovery exists only when the projection advertises the mode, requires
   separate confirmation, and states the coordinator cannot recover keys.
   History grants are a separately confirmed command bound to one object, one
   verified recipient device, an explicit epoch interval, mode, and expiry
   (≤30 days, matching the contract `maximum_days`); the
   irrecoverable-history warning renders whenever `historyRecoverable` is
   false. Metadata-only report and decrypted-evidence export are distinct
   commands; export requires a separate confirmation dialog that names the
   E2EE-boundary disclosure and carries an explicit consent version
   (`historyAndReportConsentAreSeparate`). All flows match
   `protocol/macos-encrypted-media-client-path-v1.json` (11 commands,
   recovery/report/fail-closed blocks), which the validator mutation-tests.
5. **No secret or identifier leakage.** The model/view/composition contain no
   logging, printing, UserDefaults, or file persistence (grep-verified). The
   snapshot holds only booleans, enums, display labels/titles, epoch numbers,
   and opaque validated identifiers (8–128 alnum/-/_ bytes); no key material,
   recovery payload, or decrypted bytes are representable in UI state. Every
   identifier-bearing type overrides `description`/`debugDescription` to an
   `<opaque>` constant, and commands redact identifiers; the
   `descriptionsAreRedacted` test asserts device/object/grant IDs never
   appear. The view renders titles and labels only — the source-contract test
   forbids `Text(device.id)`, `Text(grant.id)`, `Text(target.objectID)`.
6. **SwiftUI quality.** `@Observable` model with `private(set)` snapshot and a
   single `replace()` mutation point; `@MainActor` isolation; internal view
   with `@State` confirmation and `confirmationDialog(presenting:)`;
   command builders re-evaluated at confirmation time, so a stale dialog
   confirmation no-ops instead of acting on changed state. Buttons disable
   when their command is nil; every consequential action is a real `Button`
   (no `onTapGesture`, enforced by test and validator);
   `accessibilityElement(children: .contain)`, labels/values on status
   elements, and a ⌘R refresh shortcut are present. Patterns match the
   accepted sibling Pulsar surfaces (`PulsarTargetsInboxModel`,
   `PulsarShellModel` actions/copy conventions); the composition follows the
   existing `Mac*AppComposition` precedent in `Sources/NodeApp`.

## Commands and evidence (all at exact `3a64b18`)

| Check | Result |
| --- | --- |
| `xcrun swift test --filter PulsarEncryptedMediaModelTests` | 6/6 passed |
| `xcrun swift test` (full) | 356 tests, 57 suites, all passed |
| `python3 scripts/acceptance/validate_macos_encrypted_media_client_path.py` | PASS (production dark) |
| `python3 scripts/acceptance/test_macos_encrypted_media_client_path.py` | 5/5 OK (incl. 4 fail-closed mutations) |
| `python3 scripts/acceptance/run_automated.py --suite all` | 16/16 pass; manifest `.temp/acceptance/20260720T080331Z/manifest.json`, `status: pass`, head `3a64b18`, only board tracking dirty |
| `xcrun swift build -c release` | Build complete, no errors |
| Runtime-reference grep (`Sources` minus new files) | zero references — production-dark holds |
| Board resource copy `diff` vs repo copies | byte-identical |

Formatting/lint: the repository defines no Swift formatter or linter
configuration (no `.swift-format`/SwiftLint config, no Makefile lint target);
the new sources visually match the prevailing 2-space style, and no formatting
gate exists to run.

The producer's own pre-commit harness manifest
(`.temp/acceptance/20260720T074006Z/manifest.json`, `status: pass`) is
consistent with the reproduced results.

## Findings table

| # | Severity | File | Finding | Disposition |
| --- | --- | --- | --- | --- |
| 1 | Low | `PulsarEncryptedMediaModel.swift:401-403` | Tautological normalization branch (`unsupportedExclusionConfirmed` reassigned to a value it already has); dead code, no behavioral effect | Open (cosmetic; fix opportunistically) |
| 2 | Low | `MacEncryptedMediaClientPathComposition.swift:43-45` | The `ownershipUnattested` throw path is compile-verified and string-contract-tested but never executed by a test (the `NodeApp` executable target has no test target, consistent with all sibling `Mac*AppComposition` files) | Open (accepted precedent; a future runtime-wiring task must add an executable-path test before enablement) |
| 3 | Info | `PulsarEncryptedMediaModel.swift:567-576` | `availability` treats an in-flight command as `blocked` for protected paths — strictly fail-closed, momentary conservative UI during refresh | No action |

No Critical, High, or Medium findings. Findings 1–2 are non-blocking under the
acceptance rule and do not affect the fail-closed guarantees.

## Residual risks

- All cryptographic, Keychain, provider, codec, signed/notarized-app,
  VoiceOver/keyboard-on-hardware, traffic/memory/crash-forensics, and
  moderation-storage claims remain unproven by automation and are explicitly
  deferred to manual epic `EPIC-260714-th54l3`; nothing in this commit takes
  credit for them.
- The abstract ownership lease is a construction-time attestation; its real
  cross-process guarantee (e.g., an OS file lock or launchd single-instance
  contract) must be implemented and adversarially tested by the future
  runtime-wiring task before any capability advertisement.
- The `unsupportedExclusionConfirmed` flag is trusted from the server
  projection; the confirm command carries the exact excluded ID set, so the
  coordinator must validate set equality when the runtime path is wired.

## Board routing

- Verdict: `ACCEPTED` → task status `done`.
