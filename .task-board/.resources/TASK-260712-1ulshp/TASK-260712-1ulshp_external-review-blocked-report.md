> **SUPERSEDED** by `TASK-260712-1ulshp_external-implementation-security-review.md` (ACCEPTED).
> This interim report captured the transient point where Fable 5 delegation failed on a credit limit.
> The review was then completed under the board-designated Claude Opus 4.8 fallback reviewer and ACCEPTED.
> Retained for the audit trail only.

# TASK-260712-1ulshp — External cryptographic implementation review: BLOCKED (reviewer-model unavailable)

- Date: 2026-07-20
- Run: RUN-260720-191344
- Task: `TASK-260712-1ulshp` — Complete external cryptographic implementation review
- Frozen source candidate: `9d7ace6dc7337cd2191f35b0d8373228cf759398`, tree `ef819c9bd3e18e7532630510622f28e486f20007`
- Integrated review head: `909e739bcb341ced52789c4d17195fed5ed4ec53`; current branch HEAD `46e6eac` (board-only)
- Verdict: **BLOCKED — no verdict issued.** The required independent review under the
  user-approved reviewer model did not complete. This is NOT an acceptance and NOT a
  rejection. `e2ee_media` remains disabled; nothing was activated; no production
  readiness, provider selection, SBOM, Store, manual, rollout, or beta claim is made.
  `TASK-260712-yj668d`, `TASK-260712-30xwu2`, `TASK-260712-1actom` remain open and untouched.

## The blocker (external + human-only approval decision)

The task brief states the user **explicitly approved Claude Fable 5 max** as the sole
independent, non-implementing technical reviewer. The prior engineering-packet review
(`TASK-260712-1bcpda`) was likewise performed under Claude Fable 5, establishing a
deliberate **cross-model independence line** for this E2EE security gate.

This review session runs on **Claude Opus 4.8**. To honor the approved reviewer model,
the deep adversarial source review was delegated to five parallel Claude Fable 5 agents
(one per security dimension). **All five terminated on a hard account-level error:**

> `You've reached your Fable 5 limit. Run /usage-credits to continue or switch models.`

This is not a transient/retryable runtime failure (not a 429/529 backoff); it is an
account credit/quota exhaustion. The approved reviewer model is therefore unavailable
in this environment, so the full independent adversarial review across all required
dimensions and the ~22k lines of E2EE product source **could not be completed under the
approved model**.

Substituting Opus 4.8 as the **reviewer-of-record for a security sign-off** is a human
approval decision, not one this session may self-authorize: it changes the cross-model
independence property the user deliberately chose, and "approval in one context does not
extend to the next." Hence: blocked, pending an explicit human decision.

## What WAS completed this run (boundary + reproduction, all green)

These are objective, model-independent facts, independently recomputed this run:

1. **Boundary is clean.** `git diff 9d7ace6..909e739` touches only review-pack/tooling/
   board/planning: adds packet JSON, handoff doc, five review tools/tests, a 2-line
   `run_automated.py` suite registration, board lineage. **No product/runtime source and
   no dependency manifest** (checked `package.json`/`go.mod`/`go.sum`/`Package.swift`/
   `Package.resolved`/lockfiles → NONE) changed in the interval.
2. **Working tree product source == frozen candidate `9d7ace6`** (byte-identical for
   `coordinator/`, `node-app/`, `pulsar-win/`, `protocol/`), so reproduction below runs
   at the exact frozen head.
3. **Reproduction — all pass:**
   - `generate_implementation_review_packet.py --check` → `status=pass`, 128 anchors, 19 components (regenerates the packet from live git/file state and requires byte-identity).
   - `validate_cross_platform_vectors.py` → `status=pass`, 4 families, `scope=repository-fixtures-only`, `manualInteroperability=not-run`.
   - `validate_e2ee_c4_c6_review_pack.py` → `status=pass`, `externalReview=required`, `manualEvidence=not-run`, `sourceCandidate=9d7ace6…`.
   - `python3 -m unittest test_e2ee_c4_c6_review_pack test_e2ee_cross_platform_parity` → 9/9 OK.
   - `go test -race ./internal/e2eecontract ./internal/store ./internal/moderation -run 'E2EE|Opaque|Protected|HistoryGrant|Report|Recovery|Routing'` → all `ok` (store 99.8s, fresh).

## Preliminary Opus-4.8 structural sampling (NOT a completed review, NOT a sign-off)

A focused Opus-4.8 read of the crux primitives found the core security invariants
structurally present; **no Critical/High surfaced in the sampled crux.** This is a
partial spot-check only and does not cover all dimensions/files:

- **Ciphertext-only coordinator boundary — holds (sampled):** `contract.go:133`
  `coordinatorForbiddenFields = ["plaintext","content_key","epoch_secret","sender_key","recovery_secret",…]`
  rejected fail-closed via `rejectCoordinatorForbiddenFields` before routing
  (`contract.go:121,152`). `e2ee_schema.go` comments/constraints keep keys/plaintext/
  captions out of coordinator tables.
- **Production-dark is enforced, not accidental:** `contract.go:62` `ProductionConfig
  remains fail-closed until a reviewed library, suite and container [are] selected`;
  macOS send gates on `guard sealer.productionApproved || fixtureMode else …`
  (`MacProtectedMediaSend.swift:365`).
- **Nonce-reuse rejection + framing bind (sampled):** Windows send tracks a per-artifact
  `nonces` set and rejects reuse with bounds (`windows_protected_media_send.go:618-624,
  780-788`); Windows live PTT binds a strictly-incrementing `Sequence` into the frame
  header (`binary.BigEndian.PutUint32(result[36:40], …)`) and AAD, enforces
  `frame.Sequence != outgoingSequence+1` rejection and `ErrWindowsE2EELiveNonceReuse`
  (`windows_e2ee_live_ptt.go:31,63,99,355,384`); macOS enforces nonce uniqueness
  (`Set(artifact.chunks.map(\.nonce)).count == artifact.chunks.count`) and a `nonceReuse`
  live case (`MacE2EELivePTT.swift:10`).

These preliminary observations are **encouraging but not sufficient**: nonce/key/domain
separation across all four object families, membership/epoch/commit lineage, history-grant
binding, device transfer, report-consent gating, replay/fork/downgrade, and the Windows
generation-lock concurrency were NOT fully adversarially reviewed because the approved
model was unavailable.

## Options (for the human decision)

- **A — Replenish Fable 5 credits and re-run (recommended).** Preserves the deliberate
  cross-model independence line (prior packet review was Fable 5). Re-run this task; the
  five-dimension Fable 5 review completes, findings are verified, and the task routes to
  `done` (if no open Critical/High) or `development`. Cost: Fable 5 usage credits.
- **B — Explicitly approve Claude Opus 4.8 (this session) as the substitute reviewer-of-
  record.** Unblocks immediately; the review completes under Opus 4.8 and the dated report
  honestly names Opus 4.8. Tradeoff: Opus may share a model line with implementation
  agents, weakening the cross-model independence the user originally chose; acceptable only
  if the user judges non-implementation independence sufficient for this gate.
- **C — Approve a different available independent model** (e.g. a distinct non-implementing
  line) as reviewer-of-record. Same mechanics as B under whichever model is named.

**Recommendation: A** — it keeps the security sign-off aligned with the user's explicit
approval and the established cross-model independence. Fall back to B only if credits are
unavailable and the user accepts Opus-4.8 non-implementation independence for this gate.

## Exact decision / input needed from the human

> The user-approved reviewer model (Claude Fable 5 max) is out of account credits, so the
> external cryptographic review cannot complete under it. Choose one:
> (A) top up Fable 5 credits so this task re-runs under the approved model; or
> (B) explicitly approve Claude Opus 4.8 (or (C) another named independent model) as the
> substitute reviewer-of-record for this security sign-off.

## Constraints honored

- No product/runtime code modified (read-only reviewer).
- `e2ee_media` not activated; no provider/suite/container selected; no SBOM/rollout/beta/
  Store/manual/hardware claim.
- Five closure checklist items left UNCHECKED (review not completed under the approved model).
- `TASK-260712-yj668d`, `TASK-260712-30xwu2`, `TASK-260712-1actom` not closed and not touched.
