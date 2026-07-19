# Independent delta review — TASK-260712-1yz5ca (opaque protected-media and live-frame router)

- **Producer commit (exact):** `e4488ed2c0335e57910d704cf4bb4119593bbfdd` (`feat(e2ee): add production-dark opaque media router`), first parent `32fee4ac` (merged routing/rotation foundation). Reviewed HEAD matched the pinned SHA exactly.
- **Reviewer:** independent reviewer agent, model `claude-fable-5` (Claude Fable 5), review runs RUN-260719-84adbc (initial) + RUN-260719-91776a (this completion pass), 2026-07-20.
- **Verdict:** **APPROVE** — accepted as an explicitly scoped **dormant, production-dark, keyless engineering result**. No Critical or High findings. Task set to `done`.

## Review provenance

RUN-260719-84adbc ended without a verdict and its captured spawn log resource is 0 bytes. This completion pass therefore did **not** rely on the lost log: every fast check below was independently re-executed in the foreground on this pass. Per the completion brief, no long background job was launched.

## Independently reproduced in this completion pass (all green)

| Check | Result |
|---|---|
| Exact SHA match of reviewed tree | `e4488ed2c0335e57910d704cf4bb4119593bbfdd` confirmed |
| SHA-256 pin inventory, `acceptance/phase3/e2ee-opaque-media-router-v1.json` | **14/14 artifact hashes recomputed and match** |
| `cd coordinator && go vet ./...` | pass |
| `cd coordinator && go test ./...` | exit 0, 0 FAIL, 23 packages ok |
| Focused race: `go test -race ./internal/store ./internal/e2eecontract -run 'TestE2EEOpaque\|TestOpaqueLive' -count=1` | pass — store **9.752s**, e2eecontract **1.376s** |
| `python3 scripts/acceptance/validate_e2ee_opaque_media_router.py` | `PASS (production disabled)` |
| Exact acceptance-contract command from `scripts/acceptance/run_automated.py` (36-module unittest run) | **Ran 212 tests in 89.173s — OK** (expected count 212 met) |
| Exact-previous-head rollback/compat: `go test -tags previoushead -count=1 ./internal/store -run <PREVIOUS_HEAD_PATTERN>` | pass, 50.249s |

## Broad race: producer-only evidence, explicit judgment

The full-suite race run `go test -race -timeout 15m ./internal/store ./internal/e2eecontract -count=1` was **not independently reproduced**. RUN-260719-84adbc launched it as a background job and exited before completion; this pass was directed not to relaunch it. Producer evidence records it green (store **594.955s**, e2eecontract **1.460s**) with an honestly retained earlier **non-accepted** attempt that hit the default 10m timeout inside unrelated `TestTransmissionSchedulerRechecksDNDWithoutSuppressingUserMessagesOnly` during transmission schema init, with **no race diagnostic emitted**.

**Judgment: non-blocking for this production-dark delta.** The changed packages' new opaque paths were race-tested independently green twice (prior run store 10.612s / contract 1.427s per completion brief; this pass 9.752s / 1.376s), the producer broad race was green with plausible documented timing, no race diagnostic exists anywhere in the evidence trail, and the code has zero runtime wiring, so no production concurrency surface exists before the activation gates re-verify. Independent broad-race reproduction remains listed as an open follow-up item below.

## Boundary determinations (independently checked in this pass)

- **Keyless / opaque:** new router imports are stdlib only (`crypto/sha256` for digest verification, `database/sql`, `encoding/*`); no cipher/AEAD/crypto-library, suite, or container selection anywhere in the delta. Coordinator verifies public envelope fields and hashes only; it cannot decrypt payloads.
- **Production-dark / no runtime wiring:** no non-test caller of the new router entry points exists outside `internal/store`; `e2eecontract` is consumed only by `internal/store` routers; no HTTP/WS/cmd route registration; `e2ee_media_v1` exists only as a contract constant and is not advertised.
- **`BE` live frames:** distinct `BE` magic with strict header parse (version 1, reserved field must be zero) at `internal/e2eecontract/opaque_live.go:41,57`; no downgrade path to legacy plaintext `BP`. Live tables (`e2ee_opaque_live_sessions`, `e2ee_opaque_live_recipients`) persist session/recipient state only — **no ciphertext/payload column; frame bytes never persisted**.
- **Migration:** purely additive — `CREATE TABLE/INDEX/TRIGGER IF NOT EXISTS` for the 5 new tables plus immutability triggers; no DROP/ALTER/DML against legacy tables. Store diff touches only `e2ee_*` files plus 4 lines in `store.go`. Legacy media/transmission/inbox/history/cue tables untouched; feature-off behavior verified by the previous-head suite above.
- **Deletion limitation honestly stated:** delete revokes future server access and removes server ciphertext; already-copied client keys/ciphertext cannot be revoked by the coordinator — recorded in scope, packet, and ADR, not overclaimed.

## Upstream carry-overs

- **I2 (persisted-fork opaque fetch):** covered — `TestE2EEOpaqueObjectRecipientRevocationForkAndQuota` (`coordinator/internal/store/e2ee_media_router_test.go:173-189`) persists `fork_state='forked'` and asserts manifest fetch fails closed with `ErrE2EEForked`.
- **L1 (rotation reason_code determinism):** not applicable — this delta neither exposes nor consumes rotation audit reason codes (verified against the full store/e2eecontract diff).

## Findings by severity

- **Critical / High:** none.
- **Medium:** none.
- **Low / informational (non-blocking for this production-dark task):**
  1. RUN-260719-84adbc's captured spawn log resource is 0 bytes (tooling capture gap). Mitigated: all fast evidence re-reproduced independently in this pass; nothing rests solely on the lost log.
  2. Broad 15m-timeout race remains producer-only evidence (see judgment above); fold an independent broad-race reproduction into the next E2EE gate that runs long checks.

## Evidence honesty check

Producer notes and the acceptance packet explicitly mark as **unproved/open**: runtime HTTP/WS wiring, selected production cryptography (suite/container/library), physical hardware validation, real app playback, external crypto implementation review, and production activation. The non-accepted race attempt was retained and labeled as such. No dishonest claim found.

## Open gates (unchanged by this approval)

- Production suite/container/library selection and external cryptographic implementation review.
- Runtime HTTP/WS wiring, capability advertisement, and production activation (delta review required per packet `deltaReviewRequired: true`).
- Physical hardware / real app playback validation (EPC/manual).
- Independent reproduction of the broad `-race -timeout 15m` run at a future gate.
