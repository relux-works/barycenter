# TASK-260712-2q4jbu independent exact-SHA review verdict — ACCEPTED

- **Task**: TASK-260712-2q4jbu — Implement Windows E2EE verification recovery grant and report UX (windows-encrypted-media-client-path)
- **Reviewed commit**: `a5178b64cd91a5cb8300d29eac16e951b6d58f35` (parent `fa7628f461ccc4da5e7b2b89a80bb66c013b7c45`)
- **Reviewer run**: RUN-260720-f01d6a (terminal continuation of RUN-260720-88988f and RUN-260720-78713e, which completed inspection but recorded no verdict; no acceptance credit was carried from them)
- **Verdict**: **accepted** — zero open Critical, High, or Medium findings. Two Low/informational observations recorded below; neither blocks acceptance.
- **Date**: 2026-07-20

## Scope reviewed

Full producer diff `fa7628f..a5178b6` (17 files, +1815/−9): `pulsar-win/windows_encrypted_media_client.go` (857 lines) and its test (295 lines), new validator `validate_windows_encrypted_media_client_path.py`, new acceptance contract test, delta edits to the four earlier Windows validators, harness registration in `run_automated.py`, portable contract `protocol/windows-encrypted-media-client-path-v1.json`, acceptance evidence `acceptance/phase3/windows-encrypted-media-client-path-v1.json`, analysis doc, planning update, and board resources.

## Findings against the required review focus

### 1. Production-dark — CONFIRMED
- No `main.go` change in the diff; `pulsar-win/main.go` contains zero references to `WindowsEncryptedMediaClientPathComposition` or its constructor (verified by grep and by the shipped test `TestWindowsEncryptedMediaCompositionUsesOneRepositoryAndStaysRuntimeDark`, which reads `main.go` and asserts `windowsEncryptedMediaSourceIsRuntimeDark`).
- No capability advertisement, provider/suite/container selection anywhere in the diff. `ProductionDarkModel` boots with `CapabilityAdvertised=false`, `ReviewedSuiteSelected=false`, `RuntimeWiringApproved=false`, state `Loading`.
- Contract asserts `status=production-dark`, `runtime_wired=false`, `capability_advertised=false`, `selected_production_suite=null`; evidence asserts all five production flags false and `manualEpic=EPIC-260714-th54l3` with ≥7 deferred manual claims (native DPAPI, signed MSIX, real devices, audio, accessibility, traffic/memory/crash/deletion forensics). No manual evidence is claimed by this diff; manifest records `manualEvidence=not-run`.

### 2. Single-repository composition — CONFIRMED
- `NewWindowsEncryptedMediaClientPathComposition` requires exactly one non-nil `WindowsE2EEKeyStateRepository` and passes that exact pointer to the accepted `WindowsProtectedMediaSendService`, `WindowsProtectedMediaPlaybackService`, and `WindowsE2EELiveSessionFactory`. The test asserts pointer identity of `keyState` across all four.
- Generation reservation audit: only send (domain `"media"`) and live PTT (domain `"live_ptt"`) call `ReserveSendGeneration`, both through the single repository, which enforces an expected-revision compare-and-swap on the group-state payload before incrementing `SendGeneration` — no double or divergent reservation is possible across the composed paths; playback never reserves. It does not construct `newDefaultWindowsE2EEKeyStateRepository` (validator-enforced).

### 3. Fail-closed protected status and commands — CONFIRMED
- Gate chain `ReadyForCommand → RecoveryFoundationReady → ProtectedCryptographyReady → ProtectedFoundationReady` requires ready surface, no in-flight command, runtime approval, reviewed suite, capability, key-state readiness, same-repository witness, safe ownership, verified this-device, current membership, nonzero epoch, and confirmed unsupported-recipient exclusion, in monotone order.
- Normalization forces `CapabilityAdvertised=false` unless ownership is safe AND same-repository witness holds AND runtime wiring is approved AND the reviewed suite is selected; forces `UnsupportedExclusionConfirmed=false` when epoch/verification/membership regress; clears stale recovery targets, invalid drafts, and invalid report targets. Unknown states/paths collapse to `CoordinatorError`/`Plaintext`.
- Silent downgrade impossible: protected paths with unconfirmed unsupported recipients are `Blocked` (label "Blocked; no plaintext fallback"), not plaintext; `SelectPathCommand` refuses protected paths unless availability is exactly `encrypted`; plaintext is only ever an explicit selection labeled "Not end-to-end encrypted". False encrypted-ready state impossible: `Availability=encrypted` requires the full gate chain plus the per-path component readiness, symmetric with `PathFailure` codes.

### 4. Verification / recovery / grants / report consent — CONFIRMED
- Verify only unverified current members; revoke requires explicit confirmation and `CanRevoke` (lost-device revoke covered; this-device non-revocable covered).
- Device transfer requires full cryptographic readiness, an advertised recovery mode, and a verified, current, non-this-device target — current epoch access only (contract: `current_epoch_only=true`, `history_included_by_default=false`, `coordinator_key_recovery=false`).
- User-held recovery requires explicit confirmation, recovery-foundation readiness, and mode advertisement.
- History grants are bounded (object+device+epoch range, first epoch > 0, ≤30-day expiry, ≤100 grants, verified current recipient), require explicit confirmation and a valid draft; revoke targets only active grants. Irrecoverable-history warning renders whenever `HistoryRecoverable=false`.
- Metadata-only report and decrypted-evidence export are separate commands: metadata never carries a consent version and stays available when evidence export is denied; evidence export requires explicit confirmation, full cryptographic readiness, target capability + readiness, and a validated `ConsentVersion` (contract: `separate_confirmation_required=true`, disclosure flags honest in both modes).

### 5. Secret/identifier hygiene, accessibility, concurrency — CONFIRMED
- The model file imports no logging, filesystem, or network packages; the UI layer persists nothing. All sensitive structs (snapshot, device, grant, draft, report target, command) implement redacted `String`/`GoString`; the shipped redaction test asserts no device/object/grant identifier appears in any rendered description or presentation text.
- `WindowsEncryptedMediaPresentation` carries display labels and status values only — no stable identifiers, key material, or decrypted bytes. English and Russian projections both tested with non-empty accessible names/values on every row.
- Thread safety: `sync.RWMutex` around a snapshot that is deep-cloned on both `Replace` and `Snapshot`; identifier/action/consent inputs are pattern-validated with dedup and hard caps (64 devices/recipients, 100 grants). Focused `go test -race` passes; full-package race stage passed in the manifest.

### 6. Validator delta admits only this seam — CONFIRMED
- Each of the four earlier validators (`key_state`, `live_ptt`, `playback`, `send`) previously banned any production reference to their component; the delta exempts exactly `windows_encrypted_media_client.go` and replaces the blanket ban with a positive assertion that this one file contains the production-dark markers (`intentionally absent from`, `CapabilityAdvertised: false`, `RuntimeWiringApproved: false`) and, for key state, does not construct a default repository. Every other production file remains fully banned. This narrows, not weakens, the production-dark guarantee. All four validators re-run PASS at this head.

## Automated evidence (all at exact head `a5178b6`)

- Producer clean exact-head manifest `.temp/acceptance/task-260712-2q4jbu-exact-a5178b6/manifest.json`: `status=pass`, `startDirty=false`, `endDirty=false`, 16/16 stages exit 0 — 247 acceptance/contract tests, container probe test/race/amd64/arm64 builds, coordinator vet/tests/moderation contract/previous-head rollback, Windows vet/test/race/cross-vet/cross-build amd64+arm64, and 356 Swift tests in 57 suites. `scope=repository-automated-only`, `manualEvidence=not-run`.
- Manifest integrity independently verified this run: SHA-256 of logs 01, 11, 12, 16 recomputed and matched the manifest; log contents confirm `Ran 247 tests … OK`, Windows test/race `ok`, and `Test run with 356 tests in 57 suites passed` with zero failures.
- Independent synchronous spot checks this run (working tree at exact head, unmodified product source): `validate_windows_encrypted_media_client_path.py` PASS; focused acceptance suite `test_windows_encrypted_media_client_path.py` 5/5 OK; `go test -race -run TestWindowsEncryptedMedia ./pulsar-win` PASS; `gofmt -l` clean on both new files; all four delta-adjusted validators PASS.
- No credit is taken for any manual evidence; all signed-MSIX/native-DPAPI/real-device/audio/accessibility/forensics claims remain deferred to `EPIC-260714-th54l3`.

## Non-blocking observations (Low / informational)

1. **Low** — `normalizeWindowsEncryptedMediaSnapshot` does not validate `WindowsEncryptedMediaHistoryGrant.Status` against the three known values; a grant with an unknown status string survives normalization (as non-active it skips the expiry check). Impact is nil today: `RevokeHistoryGrantCommand` matches only `active` grants, presentation does not render grants, and no availability or command gate reads grant status. Worth tightening opportunistically when grant rendering is added.
2. **Info** — `Presentation` takes an independent snapshot per `Availability`/`PathFailure`/`SelectPathCommand` call, so a concurrent `Replace` mid-render could yield mixed-but-individually-fail-closed rows within one presentation. Transient and self-correcting on next render; no unsafe state is representable.

## Verdict routing

Accepted → task `done`. Reviewer DoD satisfied: implementation matches AC (honest path status, verification/revoke, transfer, user-held recovery, bounded history grants, irrecoverable-history warning, separated report consent, no silent downgrade or false E2EE state, accessibility and secret-leak tests present and green); solution fits the accepted component architecture and stays production-dark; tests green at the exact SHA with independently verified evidence.
