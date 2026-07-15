# Phase 1 root integration review

- Task: `TASK-260712-38lssj`
- Review date: 2026-07-15
- Approved planning baseline: `38ebd385e105eb2f6c7012c608cd1debfa3aad5e`
- Exact reviewed engineering candidate: `16420c2ce652d05d534fb45b5ef9a7124d4bbdd6`
- Reviewer: `codex-inline-review`
- Decision: engineering candidate accepted for reversible P2 coding; Phase 1
  product, Store and release acceptance withheld

## Decision boundary

The candidate has no unresolved critical or high engineering finding in the
reviewed source. Its deterministic unit, integration, race, migration,
rollback, contract, policy and package-shape gates are green. It is therefore
an accepted **engineering baseline** for the next coding phase.

This is not acceptance of the real application or a Store submission. No
physical Windows/macOS result, audible A3/A4 result, real screenshot, WACK
result, IARC portal answer, independent-review signature, mailbox delivery or
Partner Center outcome is inferred from repository evidence. The release
accepted-build field remains empty until those owner and manual gates finish.

## Complete diff inventory and AC mapping

The machine-readable inventory is
[`p1-root-review-manifest.json`](p1-root-review-manifest.json). It covers the
complete no-renames diff from the approved baseline to the exact candidate:

- 68 first-parent integration intervals;
- 63 task IDs represented by those intervals;
- 737 create/modify/delete path entries (698 paths with Git rename detection);
- 98,470 added and 3,818 deleted lines with renames expanded;
- zero files without a task mapping.

For every path the manifest records the candidate blob, line counts, owning
task interval(s), task acceptance-criteria hash and inherited A1-A8 scenarios.
For every task it embeds the full card acceptance criteria, current review
status and every first-parent commit. Deleted and relocated task-board files
remain present as explicit `candidate_blob: null` entries; they are not hidden
by rename detection.

The task-to-scenario mapping was re-derived from
`docs/spec-self-contained-audio.md` section 19.4 and the root amendments:

| Scenario | Engineering seams reviewed | Product evidence still required |
| --- | --- | --- |
| A1 | accountless shell, builtin cue, capture lifecycle, upload, target, receipt and Store instructions | clean installed signed Windows path and real screenshots |
| A2 | invite/credential generations, immutable target snapshots, barrier, ready/start and receipt state | two physical Pulsars and measured inter-node skew |
| A3 | pre-duck, overlay continuity, limiter order, cancellation and 100-run deterministic bounds | real provider listening, route changes and ≤200 ms measurement |
| A4 | provider anchor, interrupt ownership, resume/failure/cancel races and ≤500 ms synthetic bound | real provider audible-position and route evidence |
| A5 | immediate legacy Telegram enqueue, FIFO and atomic callback replacement | production-shaped bot/application observation if requested by manual plan |
| A6 | DND/block precedence, offline tombstones, late reconnect and exact reasons | real application/offline observation |
| A7 | resumable upload, idempotent finalize, reconnect generations, retry/delete drafts and fault injection | installed-app interruption/restart observation |
| A8 | EN/RU listing, policy hashes, certification notes, asset/WACK/IARC fail-closed validators | exact-build screenshots, WACK, portal IARC and authorized submission |

## Semantic review by risk seam

### Identity, ingest, ACL and deletion

Credential generations and recovery remain separate from node authority;
single-use/replay paths and redaction are covered on coordinator, Keychain and
DPAPI implementations. Upload tokens are scoped and expiring, offsets are
monotonic under concurrent writes, finalize is idempotent and actual bytes are
bounded. Worker inputs, demux protocols, resources and stale publication are
closed. Download authorization re-resolves owner plus immutable target
generation and holds the storage identity through open. Delete, retention,
block and cancellation converge through canonical lifecycle state.

### Transmission, history and Telegram

The coordinator owns `accepted_at`, target resolution and the playback-domain
scheduler. The three-second ready deadline and start formula are identical in
contract, store and scheduler tests. Overlay downgrade is whole-transmission;
interrupt requires confirmation and never invents a fallback. All 39 wire
goldens round-trip across coordinator, Windows and Swift. History is
ActorContext-scoped and paginated; replay creates a new transmission. Telegram
callbacks are opaque, actor/role/chat/message bound, expiring and replay-safe;
legacy voice remains immediate `after_current` and a post-start replacement is
`too_late`.

### Realtime audio and capture

Both mixers preserve `limiter(main * duck + overlay * overlay_gain + cues)`
before local master gain. Windows render-boundary tests reject allocation,
locks, waits, sleeps and goroutine creation and measure zero active allocations.
macOS source guards keep the callback free of dispatch, locks, I/O and
allocation; multi-producer gain publication is serialized outside it. Prepared
PCM stays below the 64 MiB P1 bound, generation ownership is single-finalizer,
and 100 sequential overlays pass on both implementations. Capture cues remain
outside committed samples; partial/self-test media is disposable while a
finalized unsent recording is a durable retry/delete draft.

### Security, migration, policy and Store

Public HTTP and pre-auth WebSocket admission are bounded. Forwarded pairing
identity is trusted only from the loopback TLS terminator, rate-limit state is
bounded and both Go modules select patched Go 1.25.12. Migrations use immediate
transactions, propagate column errors, serialize concurrent startup and pass
the exact predecessor matrix. Moderation has distinct credentials and scopes,
immutable evidence audit and canonical enforcement calls. Legal/public values
and policy bytes are exact-hash inputs; Store readiness fails closed on missing
screenshots, WACK, exact build, IARC export or owner decision.

## Independent findings and dispositions

All nine high findings found by the four technical self-audits were inspected
against their corrective diffs and focused regressions:

| Finding | Disposition in candidate |
| --- | --- |
| `P1-PROTO-001` | closed: both Go runtimes now reject mismatched envelope major before dispatch, matching Swift |
| `P1-AUDIO-001` | closed: Windows async mixer/resume failure and `resume_main` semantics are typed and single-finalizer |
| `P1-AUDIO-002` | closed: macOS reader ownership is atomic and gain producers serialize outside the callback |
| `P1-AUDIO-003` | closed: FIFO shutdown is interruptible and heartbeat state is one queue snapshot |
| `P1-MIG-001` | closed: legacy/orbit DDL is transactional and column failures reach startup |
| `P1-MIG-002` | closed: busy policy precedes WAL and concurrent open serializes |
| `P1-SEC-001` | closed: proxy source trust and pairing limiter state are bounded |
| `P1-SEC-002` | closed: HTTP and pending WebSocket registration resources are bounded |
| `P1-SEC-003` | closed: release modules require patched Go 1.25.12 and exact scans are clean |

The reports are technical self-audits, not independent signatures. Required
non-implementing approval remains open in `TASK-260715-3ffm3r`,
`TASK-260715-s838ym`, `TASK-260715-unbb7c` and `TASK-260715-10ksxz`.

## Verification rerun by root review

| Command or evidence | Result |
| --- | --- |
| `git diff --check 38ebd385..16420c2` | found only five Markdown hard-break whitespace locations in mirrored artifacts; normalized in the review packet, no product behavior changed |
| `GOTOOLCHAIN=go1.25.12 go vet ./...` in both Go modules | pass |
| `GOTOOLCHAIN=go1.25.12 go test -race -count=1 ./...` in `coordinator` | pass, including store in 147.112 s and command runtime in 96.898 s |
| same full race command in `pulsar-win` | pass, all four packages |
| `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test` | pass, 218 tests in 35 suites |
| exact task clean acceptance runs | 12/12 on protocol `cde0aa4`, audio `805337d`, migration `7736b75`, security `a87532c` and Store `99f1957` |
| hosted exact engineering runs | all four jobs green: `29399875529`, `29401627207`, `29402957156`, `29404910264`, `29406679102` |
| manifest regeneration and JSON parse | deterministic and valid; zero unmapped files |

The root packet itself must pass the repository-wide clean 12-stage acceptance
suite and hosted four-job CI after its commit is frozen; that exact run belongs
in the task-board outcome resource rather than being predicted here.

## Remaining risks and hard holds

1. Independent protocol, realtime, migration and security signatures are open.
2. Physical A1-A8, Windows 10/11, macOS, audible timing, screenshot and WACK
   observations remain in `EPIC-260714-th54l3`.
3. Exact IARC portal output, signed MSIX identity and owner `proceed` remain in
   `TASK-260715-24ube9`; no Partner Center mutation is authorized.
4. Mailbox delivery is not proven while `barycenter.live` has no verified MX;
   the external operations ledger owns that check.
5. NodeCore intentionally compiles in Swift 5 language mode under a Swift 6
   package and emits Sendable diagnostics around queue-owned legacy
   `PlayerCore` captures. Current synchronization and tests pass, but strict
   Swift 6 migration remains a non-blocking future hardening item.

These holds reject release acceptance but do not require reverting the
reviewed engineering candidate or stopping reversible P2 implementation.
