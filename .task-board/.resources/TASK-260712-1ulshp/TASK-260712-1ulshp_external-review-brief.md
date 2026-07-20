# Independent external implementation-security review brief

You are the sole non-implementing reviewer for `TASK-260712-1ulshp`. The user
explicitly approved Claude Fable 5 max as an independent technical reviewer.
Do not implement, refactor, format, or otherwise change product/runtime code.
Review the frozen implementation and publish only review evidence plus
task-board status/checklist/resource updates.

## Exact review boundary

- Integrated review head: `909e739bcb341ced52789c4d17195fed5ed4ec53`
  (PR #299 merge; no product/runtime delta after the source candidate).
- Frozen implemented source candidate:
  `9d7ace6dc7337cd2191f35b0d8373228cf759398`, tree
  `ef819c9bd3e18e7532630510622f28e486f20007`.
- Engineering packet producer: `26722eb040efab27c6b553f20f26b7d4dfb869bc`,
  parent `989be1f69a160ea6ae8c1c4ab5bc6cf903220358`.
- Primary machine packet:
  `acceptance/phase3/e2ee-c4-c6-engineering-review-pack-v1.json`.
- Human handoff:
  `docs/analysis/p3-e2ee-c4-c6-engineering-review-pack.md`.
- Prior packet-integrity verdict:
  `.task-board/.resources/TASK-260712-1bcpda/TASK-260712-1bcpda_engineering-packet-review-verdict-26722eb.md`.
- Clean exact packet-producer harness:
  `.temp/acceptance/task-260712-1bcpda-exact-26722eb/manifest.json`.
- Hosted PR #299 CI: Actions run `29732524910`, all four jobs passed.

First verify that `9d7ace6..909e739` contains no product/runtime or dependency
manifest delta beyond review-pack/tooling/board/planning integration. Review
the full E2EE implementation interval and its 19 component packets and 16
terminal reviews; do not reduce the audit to trusting the aggregate packet.

## Required security review

Independently inspect and adversarially reason about client key ownership,
provider boundaries, nonce/key/domain separation, device authentication,
membership and group-commit lineage, replay/fork/downgrade defenses, clip,
track, saved-cue and live framing, secure-storage abstractions, ciphertext-only
coordinator/storage constraints, grants/recovery/device transfer, report
evidence consent, metadata disclosure, deletion/revocation, mixed-version and
rollback behavior. Recompute representative hashes and rerun synchronous,
bounded representative tests. Do not start an asynchronous harness and exit
before it reaches a terminal result.

The current implementation is intentionally production-dark. Confirm rather
than erase these constraints: `e2ee_media` disabled; no production crypto
provider/suite/container selected; source-manifest inventory is not a final
build SBOM; packaged-app interoperability, OS secure-store behavior,
storage/traffic capture, moderation workflow, physical recovery, rollout and
beta evidence are `not-run` and remain in the manual/testing or later gate
tasks. A review acceptance must not activate E2EE or claim those artifacts.

## Verdict routing

Publish a dated task-owned Markdown report naming model/run independence,
exact commits and artifact hashes, review methods, every finding with severity,
reproducer, owner and retest state, residual risks and claim constraints.

- If any Critical or High finding is open, or the packet materially misstates
  security, add the report as outcome evidence, set this task back to
  `development`, leave closure checklist items unchecked, and state the exact
  required implementation/re-review path. Any protocol-affecting fix must
  reopen the design/delta-review chain.
- If there is no open Critical or High finding, every Medium is either fixed
  and retested or explicitly dispositioned with owner/claim constraint, and
  the dormant implementation is acceptable for its stated engineering scope,
  add the final sign-off and residual-risk report, check all five task
  checklist items plus reviewer DoD, and set the task `done`.
- Never close `TASK-260712-yj668d`, `TASK-260712-30xwu2`, or
  `TASK-260712-1actom`. Never claim production readiness, Store acceptance,
  manual app/hardware evidence, provider selection, final-build SBOM, rollout,
  beta, or E2EE activation.

The report must clearly distinguish independent implementation-security
acceptance of a disabled framework from production cryptographic approval.
