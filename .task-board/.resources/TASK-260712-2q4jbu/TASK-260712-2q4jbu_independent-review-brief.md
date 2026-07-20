# TASK-260712-2q4jbu independent review brief

Review the exact producer commit
`a5178b64cd91a5cb8300d29eac16e951b6d58f35` against its parent
`fa7628f461ccc4da5e7b2b89a80bb66c013b7c45` for
`TASK-260712-2q4jbu — windows-encrypted-media-client-path`.

You are the independent reviewer. Do not implement or edit the reviewed
product path. Inspect the task definition, checklist, linked resources,
implementation, tests, portable contract, acceptance evidence, prior accepted
component contracts, and planning update. Treat the reviewer brief, board
status transitions, spawn log, and any verdict file as outside the producer
diff.

Required review focus:

- Confirm this integration is intentionally production-dark: no `main.go`
  wiring, capability advertisement, production provider/suite/container
  selection, or claim that native DPAPI, signed MSIX, real devices, audio,
  accessibility, traffic, memory, crash, or deletion behavior was tested.
- Confirm the composition accepts exactly one
  `WindowsE2EEKeyStateRepository` and passes that exact repository to the
  accepted protected-send, protected-playback, and live-PTT services. Audit
  repository ownership and generation reservation for divergence or double
  reservation across the composed paths.
- Confirm protected status and runtime-sensitive commands fail closed unless
  projection, runtime, suite, capability, membership, verification, epoch,
  component, repository identity, ownership, and unsupported-recipient gates
  agree. Reject silent plaintext downgrade or a false encrypted-ready state.
- Confirm verification/revoke, current-epoch transfer, explicit user-held
  recovery, bounded object/device/epoch history grants, irrecoverable-history
  warning, and metadata-only versus decrypted-evidence consent follow the
  accepted contracts.
- Audit model, presentation, command descriptions, diagnostics, and tests for
  key material, recovery payloads, decrypted bytes, raw stable identifiers,
  unsafe logging, or persistence. Check English/Russian accessible projection
  and concurrency/thread safety.
- Verify the delta adjustments to the four earlier Windows acceptance
  validators admit only this exact dormant integration seam and do not weaken
  their production-dark guarantees.
- Reproduce focused and full Go tests, vet, race, Windows amd64/arm64
  cross-builds, task validator/tests, the complete clean automated harness,
  source/resource identity checks, and formatting as feasible. Distinguish all
  automated repository evidence from manual epic `EPIC-260714-th54l3`.

Acceptance requires zero open Critical, High, or Medium findings. Persist a
detailed exact-SHA verdict as an outcome resource and route the task according
to the explicit verdict; do not grant credit for deferred manual evidence.
