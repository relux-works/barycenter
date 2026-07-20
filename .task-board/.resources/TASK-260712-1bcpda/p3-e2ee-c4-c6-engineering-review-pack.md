# P3 E2EE C4-C6 engineering review pack

Task: `TASK-260712-1bcpda`

Status: engineering evidence and implementation-review handoff complete;
manual C4-C6 acceptance and the external implementation review are not complete

Source candidate: `9d7ace6dc7337cd2191f35b0d8373228cf759398`
(tree `ef819c9bd3e18e7532630510622f28e486f20007`)

Machine packet:
`acceptance/phase3/e2ee-c4-c6-engineering-review-pack-v1.json`

## Decision

The repository now contains a reproducible, source-linked review packet for the
complete production-dark E2EE implementation line. It does not accept C4, C5,
or C6 and it does not enable `e2ee_media`. Those outcomes still require:

- qualified independent implementation review in `TASK-260712-1ulshp`;
- packaged Windows/macOS C4-C6 execution in `TASK-260712-yj668d` under manual
  epic `EPIC-260714-th54l3`;
- mixed-fleet rollback/recovery drills in `TASK-260712-30xwu2`;
- beta/incident evidence in `TASK-260712-1actom`.

`live_ptt` remains a separate coordinator-readable capability. Repository
E2EE live fixtures do not change the product or privacy claim for ordinary
Live PTT.

## Frozen implementation interval

The packet starts at independently accepted design-review merge `4dd9612` and
ends at source candidate `9d7ace6`. It inventories the fifteen first-parent
implementation merges, each merge tree, its producer head, and the normalized
name-status digest for the full interval. The sequence is:

1. schema/epoch state;
2. coordinator routing/rotation;
3. opaque media/live router;
4. macOS and Windows key state;
5. voluntary report evidence;
6. device transfer and bounded history grants;
7. macOS send, playback, live, and integrated client path;
8. Windows send, playback, live, and integrated client path.

The packet contains nineteen component acceptance packets and their executable
contract tests, sixteen terminal independent design/delta/exact-SHA verdicts,
128 unique source/protocol/test/review/dependency anchors, and five review-tool
hashes. Every hash is recomputed by the validator. Superseded or incomplete
review runs are not represented as terminal acceptance.

## C4 repository evidence

Repository checks cover exact member/device lineage, epoch rotation,
removed-device route denial, replay and nonce state, current-epoch-only device
transfer, explicit object/device/epoch-bound history grants, grant expiry and
revoke, fork/clone/rollback failure, and fail-closed client status. Shared key
state and recovery vectors are consumed by both client implementations.

This is engineering preflight only. It does not prove that packaged clients on
two physical machines interoperate, that real lost-device state is erased, or
that native Keychain/DPAPI behavior matches the abstract adapters.

## C5 repository evidence

Coordinator migrations forbid plaintext media/key fields. The opaque object
and live routers accept bounded ciphertext, manifests, recipient lineage,
wrapped-key references, public replay state, and documented metadata only.
Tests cover digest tamper, replay, range/delete/revoke, quota, restart and
retention behavior. The threat-model metadata disclosure remains authoritative.

This is not a storage-provider or packet-capture result. A real deployed
coordinator disk, backup, traffic capture, memory image, crash report, and log
corpus have not been inspected. Those artifacts belong to manual C5 and the
external reviewer.

## C6 repository evidence

Metadata-only reports and voluntary decrypted-evidence copies are different
state transitions. Decrypted evidence requires explicit local consent, exact
report/object/actor/device/manifest/epoch/generation/revision binding, current
authorization, bounded moderation-at-rest reference, expiry/deletion, and
immutable audit. Client command models keep metadata reporting available
without granting decrypted export and never contain decrypted bytes.

This is not a packaged end-to-end moderation workflow. No provider upload,
moderator mailbox action, or physical deletion observation is claimed.

## Cross-platform fixture parity

`scripts/e2ee_review/validate_cross_platform_vectors.py` compares four
families:

- protected-send known-answer source/ciphertext/manifest/chunk/resume data;
- protected-playback chunk, bound and producer fixtures;
- opaque-live wire bytes, authenticated-data bindings and bounds;
- client paths, commands, grant/recovery/report policy and fail-closed gates.

Platform-specific differences are explicit: macOS requires a retained
cross-process ownership lease while Windows uses its share-none generation
lock; a few error labels and extra lifecycle regressions differ without
changing the common wire/fixture authority. Mutation tests prove that common
wire, ciphertext, playback, or command drift fails the parity validator.

The parity result is `repository-fixture-parity-only`. Windows→Windows,
Windows→macOS, macOS→Windows and macOS→macOS packaged interoperability remains
`not-run`.

## Dependency and supply boundary

The packet hashes the coordinator and Windows `go.mod`/`go.sum` files plus the
macOS `Package.swift`/`Package.resolved` files. This is a source dependency
inventory, not a final-build SBOM. No production cryptographic provider,
cipher suite, protected container, canonical MLS serialization, or production
decoder is selected. The earlier spikes remain a production no-go until the
external review and final build freeze select and audit the concrete supply
chain.

The external packet must add the exact signed build identity, final dependency
resolution, generated SBOM, license/advisory scan, code-signing identity and
reproducibility statement before any release recommendation.

## Reproduction

From the repository root:

```sh
python3 scripts/e2ee_review/generate_implementation_review_packet.py --check
python3 scripts/e2ee_review/validate_cross_platform_vectors.py
python3 scripts/acceptance/validate_e2ee_c4_c6_review_pack.py
python3 -m unittest scripts/acceptance/test_e2ee_c4_c6_review_pack.py scripts/acceptance/test_e2ee_cross_platform_parity.py
python3 scripts/acceptance/run_automated.py --suite all --require-clean
```

The machine packet also enumerates the nineteen component contract tests and
focused coordinator, Windows race and macOS Swift commands. An external
reviewer should run those commands on the frozen source candidate and record
every finding, reproducer, owner, fix SHA and retest. Any protocol-affecting
fix reopens the design-review gate; any source/dependency/build change after
artifact freeze invalidates the corresponding hashes.

## Residual risks and owners

- `E2EE-PACK-R01` — combined implementation review missing;
  owner `TASK-260712-1ulshp`.
- `E2EE-PACK-R02` — packaged interoperability, storage/traffic capture,
  OS secure storage and moderation workflow not run; owner
  `TASK-260712-yj668d`.
- `E2EE-PACK-R03` — production provider/suite/container/final SBOM not
  selected; owner `TASK-260712-1ulshp`.
- `E2EE-PACK-R04` — mixed-fleet rollback, loss, transfer and recovery drills
  not run; owner `TASK-260712-30xwu2`.
- `E2EE-PACK-R05` — E2EE-enabled beta and incident review not run; owner
  `TASK-260712-1actom`.

None of these risks is silently converted into an engineering pass.
