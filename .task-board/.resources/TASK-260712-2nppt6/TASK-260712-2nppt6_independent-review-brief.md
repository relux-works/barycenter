# TASK-260712-2nppt6 independent review brief

Review the exact producer commit `3a64b1808ce990fbef2cfb737839a15cbd0f6cbb` against its parent `fae94974da0dfeaa4284820070d806dd7c986b0a` for `TASK-260712-2nppt6 — macos-encrypted-media-client-path`.

You are the independent reviewer. Do not implement or edit the reviewed product path. Inspect the task definition, checklist, linked resources, implementation, tests, protocol contract, acceptance evidence, and planning update. Treat any uncommitted reviewer artifacts as outside the producer diff.

Required review focus:

- Confirm the path is intentionally production-dark: no `main.swift` wiring, concrete generation-lease implementation, capability/provider/suite/container selection, or claim that real signed/notarized hardware behavior was tested.
- Confirm all send, playback, and live-PTT services use exactly one retained `MacE2EEKeyStateRepository` and one retained abstract cross-process generation ownership lease; reject generation double reservation or divergent repository state.
- Confirm runtime-sensitive commands fail closed unless runtime, suite, capability, membership, device, epoch, repository identity, and lease checks agree. Reject silent plaintext downgrade or false encrypted-ready UI state.
- Confirm device verification/revocation, current-epoch transfer, user-held recovery, history grants, irrecoverable-history warning, and metadata-only versus decrypted-evidence moderation consent follow the accepted contracts.
- Audit UI/model state and diagnostics for Keychain secrets, key material, recovery payloads, raw stable identifiers, or unsafe logs/persistence. None may be retained or rendered.
- Check SwiftUI state ownership, accessibility, cancellation/error handling, and macOS composition lifecycle.
- Validate that automated evidence is reproducible and mutation-sensitive. Run focused and full Swift tests, the task acceptance validator/tests, full automated acceptance harness, formatting/lint, source/hash checks, and release build as feasible. Separate automated evidence from the deferred real-hardware/manual-test epic `EPIC-260714-th54l3`.

Acceptance rule: return `ACCEPTED` only when there are zero open Critical, High, or Medium findings. Otherwise return `CHANGES_REQUESTED` with precise severity, file/line evidence, impact, and required remediation; move the task to `to-dev`. Do not waive a material finding because manual hardware validation is deferred.

Write the final verdict to a task-scoped outcome resource named `TASK-260712-2nppt6_independent-review-verdict.md`, include the exact reviewed commit, commands/evidence, finding table, residual risks, and terminal verdict. Update the task-board status consistently (`done` for acceptance, `to-dev` for requested changes).
