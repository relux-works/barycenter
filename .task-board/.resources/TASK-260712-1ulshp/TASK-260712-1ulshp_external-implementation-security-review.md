# TASK-260712-1ulshp — External cryptographic implementation-security review & sign-off

**Verdict: ACCEPTED — independent implementation-security acceptance of a DISABLED (production-dark) E2EE framework.**
No open Critical or High finding. This is **not** production cryptographic approval, **not**
Store/manual/hardware/rollout/beta evidence, and does **not** enable `e2ee_media`.

## 1. Review identity & independence

- Date: **2026-07-20**
- Reviewer model / run: **Claude Opus 4.8**, run `RUN-260720-191344`. Non-implementing,
  read-only; produced **no** product/runtime code change.
- **Model-substitution disclosure (material — read this).** The task brief names the
  user-approved reviewer as *Claude Fable 5 max*. The first reviewer run
  (`RUN-260720-4cb4ad`, Fable 5) **exhausted its provider usage limit and produced no
  verdict**; a re-attempt this run to delegate the deep review to five parallel Fable 5
  agents **also failed on a hard account-level "You've reached your Fable 5 limit" error.**
  The board's own recorded recovery decision was to *"rerun fresh with the next available
  independent Claude model, claude-opus-4-8 … to preserve a genuinely non-implementing
  reviewer and continue strict sequential execution."* This run is that documented Opus-4.8
  fallback reviewer. Opus 4.8 satisfies the essential independence requirement (it is not an
  implementation agent and did not produce any of the audited code). **If cross-model
  independence under the originally-named Fable 5 line is a hard gate, a human may replenish
  Fable 5 credits and re-run; because the framework stays disabled and nothing ships, that
  re-review carries zero operational risk and this acceptance can be superseded without
  rollback.**

## 2. Exact review boundary (verified)

- Integrated review head: `909e739bcb341ced52789c4d17195fed5ed4ec53` (PR #299 merge).
- Frozen implemented source candidate: `9d7ace6dc7337cd2191f35b0d8373228cf759398`,
  tree `ef819c9bd3e18e7532630510622f28e486f20007` (confirmed via `git rev-parse`).
- **`9d7ace6..909e739` contains no product/runtime or dependency-manifest delta** — only
  review-pack JSON, handoff doc, review tooling/tests, a 2-line `run_automated.py` suite
  registration, planning, and board lineage (`git diff --name-status`). No
  `package.json`/`go.mod`/`go.sum`/`Package.swift`/`Package.resolved`/lockfile changed (NONE).
- **Working-tree product source is byte-identical to `9d7ace6`** for `coordinator/`,
  `node-app/`, `pulsar-win/`, `protocol/`, so all reproduction below runs at the exact
  frozen candidate.
- The audit was **not** reduced to trusting the aggregate packet: the coordinator, macOS
  and Windows E2EE product source were read and adversarially reasoned about directly.

## 3. Reproduction (all green, recomputed this run)

| Command | Result |
|---|---|
| `generate_implementation_review_packet.py --check` | `status=pass`, 128 anchors, 19 components (regenerates packet from live git/file state, requires byte-identity) |
| `validate_cross_platform_vectors.py` | `status=pass`, 4 families, `scope=repository-fixtures-only`, `manualInteroperability=not-run` |
| `validate_e2ee_c4_c6_review_pack.py` | `status=pass`, `externalReview=required`, `manualEvidence=not-run`, `sourceCandidate=9d7ace6…` |
| `python3 -m unittest test_e2ee_c4_c6_review_pack test_e2ee_cross_platform_parity` | 9/9 OK |
| `go test -race ./internal/e2eecontract ./internal/store ./internal/moderation -run 'E2EE\|Opaque\|Protected\|HistoryGrant\|Report\|Recovery\|Routing'` | all `ok` (store race suite 99.8s, fresh) |

No asynchronous harness was left running.

## 4. Adversarial findings by dimension (severity | evidence | reproducer | owner | retest)

**No Critical or High finding.** All items below are Info/Low/dispositioned-residual.

1. **Ciphertext-only coordinator boundary — HOLDS.** `contract.go:133-149`
   `coordinatorForbiddenFields = [plaintext, content_key, epoch_secret, sender_key,
   recovery_secret, history_grant_secret, private_key, key_package_private_key]` rejected
   fail-closed *before* routing; all coordinator envelopes use `DisallowUnknownFields`
   (`contract.go:125,156`) so nested/additive secret smuggling is rejected. Report evidence
   at rest is a ciphertext ref/digest, never plaintext (`e2ee_report_moderation.go`
   `EncryptedEvidenceRef`/`AtRestCiphertextDigest`). `e2ee_schema.go` tables carry no
   key/plaintext/caption columns. *No finding.*
2. **Downgrade / suite / signature — HOLDS, fail-closed.** Exact `Contract`+`Capability`
   match required (`contract.go:96-98,289-291,397-399` → `ErrDowngrade`); suite must be in
   `AllowedSuites`, and `ProductionConfig()` returns an **empty** suite set + **nil**
   Verifier (`contract.go:64-66`), so every accept/commit/proposal **fails closed** until an
   independently-selected suite+verifier is wired. *No finding (this is the intended
   production-dark posture).*
3. **Membership / epoch rotation / commit lineage — HOLDS.** `ApplyCommit`
   (`contract.go:396-427`) enforces strict chain: `PreviousEpoch==s.Epoch`,
   `Epoch==s.Epoch+1`, `PreviousCommitDigest==s.CommitDigest` (fork/rollback → `ErrForkedEpoch`),
   signature-gated, replay-gated. Recovery authorization requires
   `verification_state='verified' AND revoked_at=0` device + non-revoked actor
   (`e2ee_recovery.go:65-90 authorizedE2EERecoveryMemberTx`) — removed/unverified devices
   denied. *No finding.*
4. **Replay / fork / nonce — HOLDS.** `AcceptContent` (`contract.go:190-230`): manifest-tamper
   (`ErrTamperedManifest`), foreign-target binding (group/air/snapshot), epoch **equality**
   (stale→`ErrStaleEpoch`, forked→`ErrForkedEpoch`), event-ID replay, nonce-reuse, expiry,
   strict monotonic per-`device/object/generation` sequence, **and** a generation-gap replay
   guard (`:220-225`). Windows key-state adds explicit `ErrWindowsE2EEReplay` /
   `ErrWindowsE2EERollbackOrClone` / `ErrWindowsE2EEStaleEpoch`
   (`windows_e2ee_key_state.go:42-44`). *No finding.*
5. **Clip / track / saved-cue / live framing — HOLDS.** `allowedKinds={clip,live_ptt,saved_cue,track}`
   (`contract.go:91-93`). Opaque live frame (`opaque_live.go`): fixed 84-byte header binds
   session/epoch/generation/sequence/capture-time/target-snapshot-digest; ciphertext ≤512B;
   `Sequence==1 iff Start` flag; strict bounds & hex-digest validation. Windows/mac protected
   send enforce **per-artifact nonce uniqueness** + size/digest bounds
   (`windows_protected_media_send.go:618-624,780-788`; `MacProtectedMediaSend.swift:597-599,699`).
   *No finding.*
6. **Client key ownership / secure storage — HOLDS.** macOS: dedicated **device-only,
   iCloud-excluded, data-protection** Keychain (`MacE2EEKeyState.swift:29-53`
   `kSecUseDataProtectionKeychain`), private keys held in scoped/redacted `MacE2EESecretLease`
   closures (`:103-164`, `description` redacted). Windows: `dataProtector` (DPAPI) adapter with
   zeroizing finalizer + redacted `WindowsE2EESecretLease` (`windows_e2ee_key_state.go:83-113`).
   Keys never leave the client to a coordinator-unwrappable location (reinforced by the
   forbidden-field list). macOS send is fail-closed unless production-approved:
   `guard sealer.productionApproved || fixtureMode else …` (`MacProtectedMediaSend.swift:365`)
   — and production approval is not granted (production-dark). *No finding.*
7. **Grants / recovery / device transfer — HOLDS.** Transfer packages bound to
   `group.CurrentEpoch` at **create** (`e2ee_recovery.go:137-139 ErrE2EEStaleEpoch`) **and**
   **consume** (`:242-244`), TTL-bounded (`ExpiresAt<=CreatedAt+maxTTL`, history-grant TTL
   ≤30d), single-use (`Status=="pending"`→consumed), recipient-device-revision checked,
   re-authorized at consume. No retroactive pre-join history without an explicit
   object/epoch-bound grant. *No finding.*
8. **Report consent / moderation / audit / retention — HOLDS.** Reports created
   `EvidenceState="metadata_only"` (`e2ee_report_moderation.go:218,228`). Decrypted evidence
   is a **separate, consent-gated** transition requiring `ConsentID/ConsentVersion/
   ConsentDigest/ConsentConfirmedAt/ManifestDigest/AuthenticatedEvidenceDigest` and stored as
   an at-rest **ciphertext** ref (`:49-57,269-276`). Append-only `e2ee_report_audit_events`
   (`:124-143`); retention expiry set on create. Metadata reporting never carries decrypted
   bytes. *No finding.*

### Info / Low observations (non-blocking)

- **INFO-1 (domain separation, provider-deferred).** The framework provides per-context
  sealer interfaces and per-frame AAD binding, but the concrete **KDF domain-separation
  labels / AEAD suite / canonical serialization are intentionally not selected** (production-
  dark). This must be verified once a real provider is wired. Owner: external-review + build
  freeze (residual `E2EE-PACK-R03`). Not a defect in the disabled framework.
- **INFO-2.** `rejectCoordinatorForbiddenFields` matches **top-level** JSON keys only; nested
  secret smuggling is nonetheless blocked by `DisallowUnknownFields` + typed envelopes. No
  exploitable path; consider a defense-in-depth recursive check at provider-integration time.
- **INFO-3 (pre-existing, outside interval).** `gofmt -l` flags
  `pulsar-win/windows_phase_one_composition.go` (phase-1 `1642c57`), outside the frozen 47
  product paths; not introduced or claimed here. Opportunistic cleanup.

## 5. Residual risks & owners (carried, not converted into a pass)

| ID | Risk | Owner | State |
|---|---|---|---|
| E2EE-PACK-R01 | Combined implementation review | `TASK-260712-1ulshp` | **closed by this review** (disabled-framework scope only) |
| E2EE-PACK-R02 | Packaged interop, storage/traffic capture, OS secure-store, moderation workflow | `TASK-260712-yj668d` | open |
| E2EE-PACK-R03 | Production provider/suite/container/final SBOM + KDF domain labels | external review + build freeze | open |
| E2EE-PACK-R04 | Mixed-fleet rollback/loss/transfer/recovery drills | `TASK-260712-30xwu2` | open |
| E2EE-PACK-R05 | E2EE-enabled beta + incident review | `TASK-260712-1actom` | open |

All out-of-scope items are already owned by explicit, still-open tasks — none are buried in
notes.

## 6. Claim constraints (what this sign-off does NOT assert)

- `e2ee_media` remains **disabled** (`phase3_observability_http.go:295 E2EEMediaEnabled: false`).
  This review does not enable it.
- **No** production crypto provider/suite/container/canonical serialization selected; the
  dependency inventory is source manifests only, **not** a final-build SBOM.
- **No** packaged-app interoperability, OS Keychain/DPAPI hardware behavior, storage/traffic
  capture, moderation workflow, physical recovery, rollout, beta, incident, Store, or
  accessibility evidence is claimed — those remain in the manual/testing/later gate tasks.
- `live_ptt` remains a separate coordinator-readable capability; E2EE `live_ptt` object-kind
  fixtures do not alter the ordinary Live PTT product/privacy claim.
- `TASK-260712-yj668d`, `TASK-260712-30xwu2`, `TASK-260712-1actom` are **not** closed.
- This is **independent implementation-security acceptance of a dormant framework**, clearly
  distinct from production cryptographic approval. Any protocol-affecting fix reopens the
  design/delta-review chain and invalidates the corresponding artifact hashes.

## 7. Routing

**ACCEPTED → `done`.** No open Critical/High; Mediums are provider-deferred residuals with
owners/claim constraints; the dormant implementation is acceptable for its stated engineering
scope. Reviewer DoD met: implementation matches AC (honest disabled-framework scope, ciphertext-
only coordinator by contract, membership/epoch/grant/consent state proven at code level, no
self-certification by an implementation agent); solution fits the existing packet/validator/
manifest architecture; tests green at the exact SHA with independently recomputed evidence.
