# Phase 3 root line review

- Task: `TASK-260712-3g0axs`
- Review date: 2026-07-17
- Phase 2 handoff baseline: `b02538f201cdfe40fd4bbfb5150842fd96754861`
- Exact reviewed non-E2EE source candidate: `d94f51644a3acf37601b4a869b4247380372f9ec`
- Exact reviewed source tree: `4e4cca878db806650eda6f1e1642051b87a18b93`
- Reviewer: `codex-inline-reviewer`
- Decision: non-E2EE engineering baseline accepted for reversible continuation;
  production, promotion, E2EE, independent, manual and beta acceptance withheld

## Decision boundary

The complete implemented Phase 3 non-E2EE source line has no unresolved
critical or high repository-engineering finding in the exact candidate. Live
PTT, soundboard/automation, capture-quality and Phase 3 gate/observability
code can continue to the next strict-sequence engineering task.

This is not a production or whole-epic acceptance. Four E2EE preparation
intervals are inventoried but excluded under `EPIC-260716-3qsztl`; no E2EE
capability is accepted. Real-app/hardware evidence in `EPIC-260714-th54l3`,
independent reviews, rollout drills and beta soak are not run. Production build,
package and runtime hashes remain `null`.

The fail-closed decision is
[`root-line-review-v1.json`](../../acceptance/phase3/root-line-review-v1.json).

## Complete diff inventory

The deterministic
[`p3-root-review-manifest.json`](p3-root-review-manifest.json) covers every
first-parent interval and no-renames path from the Phase 2 handoff through the
exact reviewed source candidate:

- 75 first-parent intervals: 65 reviewed non-E2EE, eight deferred E2EE and two
  explicit repository-context intervals;
- 32 reviewed non-E2EE tasks and four inventoried deferred E2EE tasks;
- 420 changed paths, 66,787 added lines, 937 deleted lines and 1,700 aggregate
  interval hunk headers;
- zero paths without interval ownership.

Every interval has a patch SHA-256, hunk/line counts, task owner and C1-C7 or
section-gate mapping. Every path records its candidate blob and all owners.
Every task embeds the full card scope and acceptance criteria with hashes. The
validator regenerates the manifest and byte-compares it with the committed
artifact.

Classifications are 76 coordinator, 63 Windows client, 45 macOS client, 15
protocol/golden, 59 verification/contract, 42 operational/product docs and 120
governance paths.

## Semantic review by risk seam

### Live PTT

The coordinator owns a bounded session and target set, validates authenticated
binary frames before relay, keeps per-target nonblocking queues and retains no
audio after fanout. Generation, deadline, rate, duration, target-policy and
restart boundaries fail closed. Windows derives `audible_started` from actual
render consumption. The review corrected macOS parity: route activation no
longer claims audibility; an atomic render-frame counter now gates the one-shot
receipt off the render callback. Reject payloads are validated again at the
runtime mutation boundary.

### Soundboard and automation

Saved cues resolve only eligible canonical media or the fixed built-in cue.
Control mutations require current primary authority, CAS/idempotency proof and
content-policy acceptance. Principal secrets are stored only as versioned
hashes and shown once; replay projections remove them. Scoped triggers enforce
cue/audience/selector scopes, quiet hours, global/principal rate and concurrency
limits, target capability, canonical Air policy and ordinary transmission
recipient ceilings. Scheduler evaluation never catches up missed wall-clock
minutes. Revoke, disable, history, Telegram and desktop controls converge on
the same stored execution/transmission state.

### Capture quality and diagnostics

Protocol mirrors use the same bounded enum/state contract and generation guard.
Absent or unverified native effects remain degraded rather than inferred from
API activation. Client DSP has bounded gain/slew and applies the -3 dBFS ceiling
last. Capture ownership, cancellation and draft separation remain unchanged;
diagnostic metadata cannot open a microphone. Newly added hub diagnostics now
emit only fixed validated labels without raw orbit, node, generation or
arbitrary decoder errors.

### Gates and observability

The public health surface remains coarse. The authenticated moderation view is
fixed-cardinality and contains no tenant selectors or user-controlled labels.
Enabled missing, stale, contradictory or unsafe telemetry fails closed;
optional-disabled features do not degrade the base service. E2EE is explicitly
`deferred_unavailable`, and client timing, manual campaigns and beta evidence
remain `not_run` instead of becoming zero-valued passes.

### Authorization, resources, rollback and dependencies

HTTP methods, exact paths, content types, body bounds, origin/cookie rejection,
bearer capabilities and idempotency/CAS checks were traced to store mutations.
New queues, maps, packet windows, PCM rings, request bodies, histories and
runtime scans have explicit ceilings. Phase 3 adds no production dependency
or package-manager change. Live PTT stays env-only/default-off; accepted source
does not imply feature rollout. No destructive migration or rollback claim is
introduced by this review.

## Findings and disposition

| Finding | Severity | Disposition |
| --- | --- | --- |
| `P3-ROOT-001` macOS emitted `audible_started` at route activation | High | fixed and re-reviewed; receipt requires render-consumed frames |
| `P3-ROOT-002` new live/capture logs included raw identifiers or arbitrary errors | High | fixed and re-reviewed; fixed low-cardinality fields plus hostile-canary tests |
| `P3-ROOT-003` runtime reject path omitted defense-in-depth payload validation | Medium | fixed and re-reviewed before mutation |
| `P3-ROOT-004` new log fixture raced under `go test -race` | Low | mutex-protected capture added; targeted and full race rerun |

There are no unresolved critical or high findings in this root-reviewed
non-E2EE source candidate. The root reviewer is not represented as any required
independent reviewer or approver.

## Verification on the exact source candidate

| Command | Result |
| --- | --- |
| `cd coordinator && GOTOOLCHAIN=go1.25.12 go vet ./... && GOTOOLCHAIN=go1.25.12 go test -race -count=1 ./...` | pass |
| same full vet/race command in `pulsar-win` | pass, all four packages |
| `cd node-app && DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test` | pass, 308 tests in 52 suites |
| acceptance contract discovery | pass |
| manifest regeneration and root validator | deterministic, zero unmapped paths |

The first coordinator race attempt correctly failed on `P3-ROOT-004`; that
failed run is not acceptance evidence. Only the post-fix exact-candidate rerun
is recorded as passing.

## Open holds and strict next step

1. E2EE remains deferred to `EPIC-260716-3qsztl`; C4-C6 are not accepted.
2. Real Windows/macOS hardware, acoustic, packaged-app and accessibility work
   remains `not-run` in `EPIC-260714-th54l3`.
3. External security, realtime, automation and privacy/Store reviews are still
   required and cannot be self-approved by this session.
4. Rollout/recovery drills, beta soak and final promotion evidence are not run.
5. No production build, package or runtime configuration is accepted.

The strict next task is `TASK-260712-1ulshp`.
