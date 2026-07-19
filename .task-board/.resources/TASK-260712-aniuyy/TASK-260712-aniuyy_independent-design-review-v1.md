# Independent E2EE cryptographic design review — terminal verdict

- Task: `TASK-260712-aniuyy` (gate for `EPIC-260716-3qsztl` / `STORY-260716-1qp4gp`)
- Reviewed under: `independent-design-review-brief.md` and `p3-root-review-amendments.md`
- Review date: 2026-07-19
- Reviewer: implementation-independent design reviewer (board role `[reviewer] reviewer (claude)`, spawn run `RUN-260719-1bbaa7`). The reviewer authored none of the reviewed artifacts and has read-only access; no production code was modified. This is the design-review gate only — it is **not** the later external crypto implementation review `TASK-260712-1ulshp`, which remains a separate, human-gated input.

## VERDICT: APPROVE (design gate passed; implementation remains blocked)

- Zero open critical or high **design** findings against the reviewed documents, state machines, vectors, or models. New review findings are low/informational and tracked below with owners.
- All critical/high findings from the two no-go spikes and the audit packet **remain explicit and open by design**, exactly as the review contract requires; each has a disposition, owner, and retest condition below.
- Per the packet handoff (`reviewMayAuthorizeImplementation: false`), this approval does **not** authorize implementation: no production library, suite, serialization, or container is selected; `e2ee_media_v1` remains unadvertised and off; plaintext fallback remains forbidden.
- **Delta-review rule:** this sign-off binds exclusively to the exact hashes below. Any change to any pinned document, vector, or model file — or any protocol-affecting change elsewhere — invalidates this review and requires a delta review before the affected artifact is relied upon.

## 1. Reviewed baseline and reproduced hashes

- Git HEAD at review: `7e6c8bee735345df8e094ccfe757910c146118ba` (branch `review/task-260712-aniuyy`; working tree clean apart from `.task-board/` bookkeeping).
- Producer merge `43a4d4e1b6f717a8c36910e8781153d615d43740` is an ancestor of HEAD; no pinned path changed between that merge and HEAD.
- Every SHA-256 recomputed locally (`shasum -a 256`) and compared with `acceptance/phase3/e2ee-protocol-key-lifecycle-v1.json`. **All 12 packet pins reproduce exactly.**

| File | SHA-256 (recomputed) | Packet pin |
|---|---|---|
| protocol/e2ee-media-audit-v1.json | 52b92b2f51bf6852fb9c7c7279c379fa2bbbaa9c262dba94a2c42239b6104b32 | match |
| protocol/e2ee-media-audit-v1-vectors.json | 2264448a94cf01b5841f722089711ae8338eaf2c34bf512c51b062e4eb626a95 | match |
| acceptance/phase3/e2ee-threat-model-v1.json | a0d981e077d2b22b76ba21b6efb5fa8f680467c0337d758365a95aea9868a66d | match |
| acceptance/phase3/protected-media-container-spike-v1.json | 320eb4221598e2a607e80614e349d15d447d92141f501567161b7a779f7492bb | match |
| acceptance/phase3/group-crypto-library-spike-v1.json | da518ec18d99449b87995268a18d6e7de934ea766aac6c948b127aaa60af8860 | match |
| coordinator/internal/e2eecontract/contract.go | 6e6ba3b2861036a6bf21d011d2f496c9a8b7b253d3ba21d59930c61a4f0f1987 | match |
| coordinator/internal/e2eecontract/contract_test.go | f94c939245ffea273591e7928b57ccdac8dcb0e816e0e164bd86148cb27cd280 | match |
| pulsar-win/e2ee_contract_audit.go | 73b2ee8b2304b1648c6a1394eec24068281f84927774526585b35990a81378b9 | match |
| pulsar-win/e2ee_contract_audit_test.go | 2f6791f7dc3431d10cec357d37e0d2a641c2440c37dd3de4060c2acf2ddc4d71 | match |
| node-app/Sources/NodeCore/E2EEAuditContract.swift | 3c03962b38e78298c6ae6f2a591d6714b513652337e80322123804abf0f3f061 | match |
| node-app/Tests/NodeCoreTests/E2EEAuditContractTests.swift | 59c5ef673385e299bbd70a524de937ad5bf22e8995145b1a81e6600e1af08f1c | match |

Additional pins recorded by this review (not hashed inside the packet):

| File | SHA-256 (recomputed) |
|---|---|
| acceptance/phase3/e2ee-protocol-key-lifecycle-v1.json (packet itself) | 54255ef7307d679db97f1c6a219ebb3eff7556d2fe1e34115b4639ac7917a205 |
| docs/analysis/p3-e2ee-protocol-key-lifecycle-contract-v1.md (ADR) | c3c34aa0047fb3a4149a92a32e035af8fac1cb69ad9056ee22de09bfde884109 |

## 2. Reproduced automated evidence (all green, fresh runs)

Host: macOS (Darwin 24.6.0), go1.26.0 darwin/amd64 host toolchain (modules pin `go 1.25.12`), Apple Swift 6.2.3 (target x86_64-apple-macos14.0), Python 3.9.6.

1. Coordinator: `go test -count=1 ./internal/e2eecontract` → `ok relux.works/duet/coordinator/internal/e2eecontract 0.392s` (valid fixture accepted; production config rejects the valid fixture as `unknown_suite`; all 10 shared malformed vectors; secret/unknown-field rejection; commit rotate/replay/fork/stale).
2. Windows model: `go test -count=1 ./...` in `pulsar-win/` → ok for `pulsar-win`, `cmd/pulsar-win-probe`, `internal/winprobe`, `wire` (shared vectors, production no-go, commit ordering/fork, coordinator-secret rejection).
3. macOS model: `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter E2EEAuditContractTests` → 3/3 passed (`sharedMalformedVectorsFailClosed`, `coordinatorVisibleDecoderRejectsSecretsAndUnknownFields`, `sharedCommitVectorOrdersEpochAndRejectsFork`).
4. Acceptance: `python3 -m unittest scripts.acceptance.test_e2ee_protocol_key_lifecycle` → 3/3 OK. The validator itself recomputes every packet SHA-256 against the working tree, requires `capabilityAdvertisementAllowed=false` and `productionSuites=[]`, requires the forbidden-field boundary, and fails closed when enablement is mutated (both fail-closed tests reproduced).

All three implementations consume `protocol/e2ee-media-audit-v1-vectors.json` directly at the pinned hash; the same valid content, valid commit, and ten single-mutation malformed vectors produce the expected failure codes on every platform.

## 3. Scope assessment (design level)

- **Device authentication.** Coordinator-issued identity is explicitly insufficient (threat model E2EE-004/005, trust role `coordinator-authentication-service`); the verified claim is blocked until an independently reviewed verification/consistency mechanism exists (GCL-008 open, owned downstream). Sound and honest; consistent with RFC 9750's identity-compromise analysis.
- **Coordinator trust and equivocation.** Coordinator is malicious-for-content in scope; it routes ciphertext and enumerated metadata only, never creates/unwraps/escrows secrets; strict decoders reject secrets and unknown fields. Commit chaining (`previous_commit_digest` + exactly-one-epoch advance) makes forks fail closed and equivocation detectable; withholding/DoS is an accepted, disclosed residual (RR-01). Sound.
- **Group lifecycle.** Single-use key packages, client-authored proposals/commits, welcome, epoch confirmation, offline restore-before-decrypt, and rotation on leave/remove/revoke/recovery before later content. Matches RFC 9420 semantics without selecting a library. Sound.
- **Forward secrecy claims.** Claims are gated and deletion-limited (E2EE-011/020); removed members lose future epochs but retained copies are honestly excluded; RR-07 covers retained-ciphertext exposure. No overclaim found.
- **Concurrent membership.** Serialization by previous commit digest; stale and forked commits fail closed and losers must rebase from authenticated state. Modeled and vector-tested (fork + replay everywhere, stale additionally in the coordinator suite). Sound.
- **Nonce and key separation.** Unique nonce tuples per key across devices/crash/restart with deterministic domain separation (E2EE-009); container probe demonstrates HKDF domain-separated keys and reserved-counter nonce layout; models enforce nonce uniqueness and monotonic sequence (see IDR-002). Production nonce discipline is explicitly deferred to the selected suite. Sound at design level.
- **Chunk and live framing.** Seekable chunked AEAD with per-chunk AAD binding (domain, header hash, index, length), authenticated range/resume boundaries, and an honest statement that whole-container replay needs protocol state, not AEAD. Live PTT: epoch-bound session key, monotonic per-generation frame sequence, unique nonces, authenticated-before-jitter-decode, terminal rotation, no generation reset on reconnect. Sound.
- **Recovery and history grants.** No automatic history; grants are recipient-device-, object/range-, epoch-, target-, and expiry-bound; recovery requires a surviving authorized device or user-held capability; coordinator escrow forbidden; irrecoverability accepted and disclosed (RR-05). Sound.
- **Report evidence.** Voluntary, reporter-selected local decrypt and export; visible boundary exit; purpose limitation, least-privilege ACL, audit, short retention (E2EE-016, RR-06). No coordinator/moderator decrypt path exists. Sound.
- **Metadata disclosure.** Coordinator-visible metadata is exhaustively inventoried and matched by the routing envelope's field list; encrypted-or-local-only fields enumerated; claim rules force disclosure language. Traffic analysis accepted and disclosed (RR-01). Sound.
- **Downgrade.** Capability never advertised; production suite set empty rejects even the valid audit fixture as `unknown_suite` (reproduced on all three platforms plus the acceptance validator); the `downgrade` vector rejects capability substitution; mixed-version targets fail closed with no silent plaintext fallback. Sound.
- **Deletion limits.** Best-effort deletion cannot revoke plaintext/keys already copied; forbidden-claims list blocks "deletion erases every copy". Sound.
- **Library supply chain.** No third-party cryptographic dependency exists in any reviewed module: `coordinator/go.mod` (gorilla/websocket, x/sys, yaml.v3, modernc sqlite + indirects), `pulsar-win/go.mod` (go-winio, go-ole, gorilla/websocket, go-wca, x/net, x/sys, x/text), `node-app` (Yams 5.4.0, Sparkle 2.9.4, pinned in Package.resolved) — these manifests are the effective SBOM of the reviewed audit-only code. Candidate stacks (OpenMLS 0.8.1, mls-rs 0.55.2, mlspp) are pinned to exact commits/licenses/audit status in the spike and all remain no-go; the full production SBOM obligation stays open under GCL-010/EPC-001. Sound.

**Dormancy and boundary verification (reproduced by grep at HEAD):** `coordinator/internal/e2eecontract` has no importer outside its own package; `pulsar-win` audit symbols have no non-test call site; `E2EEAudit*` has no reference in `node-app/Sources` outside the contract file; the string `e2ee_media_v1` appears only in the three contract files, the acceptance validators, and a negative comment in the container probe ("must not be registered as e2ee_media_v1"). No runtime path advertises or handles the capability.

## 4. Findings register

### Pre-existing gate findings (verified to remain explicit; not design defects)

| ID | Sev | Status & disposition | Owner | Required fix | Retest |
|---|---|---|---|---|---|
| EPC-001 | critical | OPEN **by design** — candidate-neutral contract is the accepted decision; blocks production, not this gate | Implementation chain of EPIC-260716-3qsztl (selection before TASK-260712-3w1cst work is relied on in production); external review TASK-260712-1ulshp | Select exact audited MLS library/provider/suite/canonical serialization clearing every GCL gate | Repeat spike gates on the exact selected stack + delta design review of new hashes |
| EPC-002 | critical | OPEN **by design** — same treatment as EPC-001 | Container/codec selection tasks per PMC exit | Reviewed production container + codec for clip/track/saved-cue/live paths | Cross-platform vectors + delta review |
| EPC-003 | high | **CLOSED by this review** — TASK-260712-aniuyy has now run; evidence in §1–§3 | this review | — | — |
| EPC-004 | high | OPEN **by design** — no real KAT/interop/signed-package/hardware evidence is claimed here | EPIC-260714-th54l3; TASK-260712-1bcpda | Run real cross-platform, signed-package, persistence, recovery, hardware evidence | C4–C6 packet |
| EPC-005 | high | OPEN **by design** — implementation-specific storage/memory/deletion review cannot run against audit-only models | TASK-260712-25dzp4, TASK-260712-1x9ruo, TASK-260712-1ulshp | Secure storage, crash/log/backup exclusion, memory lifetime, deletion behavior | Implementation review on real key-state code |

All GCL-001…010 and PMC-001…008 spike findings were re-read and remain explicit and open in the pinned documents; none has been silently closed or weakened. This is the state the review contract requires.

### New findings from this review (none critical/high; none block the gate)

| ID | Sev | Finding | Owner | Required fix | Retest | Disposition |
|---|---|---|---|---|---|---|
| IDR-001 | low | Failure-code precedence for multi-fault inputs is not pinned and already diverges: coordinator checks `invalid_signature` before `tampered_manifest` (contract.go validateCommon → AcceptContent), while Windows and macOS check `tampered_manifest` first. Single-mutation vectors cannot detect this; all divergent paths still fail closed. | TASK-260712-3w1cst | Pin canonical check precedence in the schema and add at least one dual-fault shared vector | Extended shared vectors green on all three platforms | open-tracked, non-blocking |
| IDR-002 | low | The Windows audit model omits the per-sender monotonic `sequence` rule that the coordinator and macOS models enforce (`lastSequences`); the shared vector set has no sequence-regression or generation-reset vector, so the omission is invisible to the shared suite. The rule itself is normative in the protocol authority `stateRules`. | TASK-260712-3w1cst (vectors); TASK-260712-25dzp4 (Windows key state) | Add sequence-regression and generation-reset vectors; implement the rule in Windows key state | Shared vector suite green on all three platforms | open-tracked, non-blocking |
| IDR-003 | low | Strict coordinator-envelope decoding (unknown-field + forbidden-field rejection) is demonstrated only for the content routing envelope; commit envelopes are decoded in tests without an equivalent strict coordinator-visible decoder, though the design mandates strict rejection for the whole boundary. | TASK-260712-3w1cst | Extend strict decode + forbidden-field tests to commit, proposal, welcome, key-package, and history-grant envelopes | Schema forbidden-field tests green | open-tracked, non-blocking |
| IDR-004 | info | Commit stale-epoch rejection is vector-tested only in the coordinator suite; Windows/macOS commit tests cover valid/replay/fork. Behavior is implemented identically on all three. | Platform key-state tasks | Mirror the stale-commit case in platform suites | Platform suites green | open-tracked, non-blocking |
| IDR-005 | info | The packet's `baselineCommit` (73bdc18d) predates the pinned files (added at merge 43a4d4e1), and the noted producer commit 13df61df is not reachable from HEAD (pre-merge branch head). Exact-hash pins carry the integrity guarantee, so this is cosmetic. | TASK-260712-1bcpda packet conventions | Pin the merge commit that actually contains the pinned files in future packets | Next packet | open-tracked, non-blocking |

### Medium and residual risks — product language and owners

No medium findings are open. Residual risks RR-01…RR-08 in the threat model each carry an explicit disposition, and the required product language exists in `claimRules` (`requiredLimitations` + `forbidden`), including: Telegram uploads are not E2EE; deletion cannot erase obtained copies; history may be irrecoverable without a surviving device or user-held capability; metadata remains visible as documented. Owners: honest-UX tasks `TASK-260712-2q4jbu` / `TASK-260712-2nppt6` (surface the language), threat-model owner (inventory accuracy), retention/ops via RR-07, implementation reviews via RR-08.

## 5. Explicit non-claims

- No real-app, signed-package (MSIX / notarized macOS), physical-hardware, real-crypto interop, or beta evidence is claimed by this review; all remain `not-run` in EPIC-260714-th54l3 and later gates.
- No production library, cipher suite, serialization, container, or codec is selected or endorsed; the fixture suite label and injected verifier are not cryptography.
- This review does not substitute for the external crypto implementation review `TASK-260712-1ulshp` or any other independent review listed in the root amendments.

## 6. Sign-off

The design at the exact hashes in §1 is APPROVED with zero open critical/high design findings. Implementation stays blocked pending downstream selection tasks and their reviews; `e2ee_media_v1` stays off. Any protocol-affecting change invalidates this verdict and requires delta review against new pinned hashes.
