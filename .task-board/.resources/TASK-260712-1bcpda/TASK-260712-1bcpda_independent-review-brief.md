# TASK-260712-1bcpda independent review brief

Review exact producer commit
`26722eb040efab27c6b553f20f26b7d4dfb869bc` against parent
`989be1f69a160ea6ae8c1c4ab5bc6cf903220358` for
`TASK-260712-1bcpda — e2ee-c4-c6-evidence-review-pack`.

You are the independent engineering-packet reviewer. Do not edit the reviewed
packet, tooling, product code, or claims. Inspect the task, checklist, linked
sequence diagram, producer diff, source candidate, all packet resources,
component packets, terminal verdicts, tests, generator, validator, parity
checker, documentation, board boundary, and planning update. Treat reviewer
brief/status transitions/spawn logs/verdict as outside the producer diff.

This review may accept the reproducible engineering evidence pack. It must
**not** self-certify external implementation review `TASK-260712-1ulshp`,
manual C4-C6 `TASK-260712-yj668d`, rollback/recovery drills, beta, production
crypto selection, or feature activation.

Required focus:

- Confirm the frozen implementation source candidate is exactly merge
  `9d7ace6dc7337cd2191f35b0d8373228cf759398`, tree
  `ef819c9bd3e18e7532630510622f28e486f20007`, and the packet's fifteen
  first-parent post-design merges reproduce with exact trees/producer heads.
  Confirm `9d7ace6..26722eb` changes no product/runtime source.
- Recompute the implementation name-status digest, all 19 component packet
  and test hashes, all 16 terminal independent review hashes, all 128 source
  anchors, dependency manifests, review tooling and handoff-document hashes.
  Reject stale/superseded/incomplete review runs represented as terminal.
- Run all component validators/tests and audit the new packet validator's
  mutation sensitivity. Confirm source, review, dependency, C4-C6, external,
  manual, feature flag, crypto selection, pairing and residual-risk mutations
  fail closed.
- Audit cross-platform parity independently. Shared protected-send
  known-answer ciphertext/manifest/chunk/resume data, protected-playback
  fixtures, opaque-live bytes/AAD/bounds, and client command/policy gates must
  match. Platform-specific ownership/error/lifecycle differences must remain
  explicit. Repository fixture parity must not be mislabeled as packaged-app
  interoperability.
- Confirm C4 evidence covers deterministic membership/rotation/revoke,
  current-epoch transfer and explicit history grant state only; C5 covers
  ciphertext-only schemas/router constraints and disclosed metadata only; C6
  covers explicit report/evidence consent, retention/delete/audit state only.
  Reject any real storage/traffic capture, OS secure-store, physical device,
  packaged moderation, deletion, audio, accessibility, recovery or
  interoperability claim.
- Confirm `e2ee_media` remains absent/disabled and separate from ordinary
  coordinator-readable `live_ptt`; production provider/suite/container and
  final-build SBOM remain unselected. Confirm all five residual risks retain
  explicit owners and activation still requires external/manual/drill/beta
  gates.
- Reproduce focused component/mutation tests, coordinator and Windows race
  subsets, focused Swift tests, generator check, parity validator, clean exact
  16-stage automated harness, JSON/resource identity, formatting/lint and
  source scans as feasible. Use only synchronous test invocations or consume
  the complete clean exact manifest at
  `.temp/acceptance/task-260712-1bcpda-exact-26722eb/manifest.json`; do not exit
  while an asynchronous harness is still running.

Engineering acceptance requires zero open Critical, High, or Medium finding.
Persist a detailed exact-SHA verdict as an outcome resource, complete the four
reviewer DoD items, and route the task. An ACCEPTED engineering-packet verdict
must still leave `TASK-260712-1ulshp` and all manual gates open.
