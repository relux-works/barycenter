# Phase 2 root integration review

- Task: `TASK-260712-1kfnpu`
- Review date: 2026-07-16
- Approved Phase 1 handoff baseline: `c0e0d4a5686ebac897dfa2665ee2cd59ba35e133`
- Exact reviewed Phase 2 source candidate: `5f2f7e97a343b4bca84fe56ee57dd02458265f31`
- Exact reviewed source tree: `4a03b4d3a3db062ed210e6696869366a9b6cf775`
- Reviewer: `codex-inline-reviewer`
- Decision: engineering baseline accepted for reversible continuation; Phase 2
  production, promotion, B1-B7 and beta acceptance withheld

## Decision boundary

The complete Phase 2 source line has no newly unresolved critical or high
repository-engineering defect in the reviewed candidate. Deterministic schema,
migration, protocol, ACL, lifecycle, range, accounting, client-state and
contract checks pass. This source tree is therefore accepted as the exact
engineering baseline from which the next strict-sequence coding task may
continue.

This decision does not select a production codec/player combination and does
not accept a binary, package, runtime configuration, physical result or beta
window. The codec ADR remains `no-go`; production capability advertisement and
decoder registries remain disabled. The accepted production build, package and
configuration hashes are explicitly `null`. Thirteen inherited High findings
remain production-blocking and are not relabelled by this review.

The fail-closed machine-readable decision is
[`root-integration-review-v1.json`](../../acceptance/phase2/root-integration-review-v1.json).

## Complete diff inventory and task mapping

The deterministic
[`p2-root-review-manifest.json`](p2-root-review-manifest.json) covers the full
no-renames diff from the Phase 1 handoff to the exact reviewed candidate:

- 102 first-parent integration intervals;
- 50 Phase 2 task IDs, including this root review correction interval;
- two explicit non-Phase-2 repository-context intervals retained rather than
  silently attributed to the epic;
- 624 create/modify/delete path entries, 84,546 added lines and 2,172 deleted
  lines;
- zero paths without interval ownership.

Every path records its candidate blob, line counts, classification, owning
task intervals and inherited B1-B7/section gates. Every task embeds the full
card acceptance criteria and its digest. The manifest is regenerated from Git
and byte-compared by the root-review validator, so a changed commit, tree,
mapping or file inventory fails closed.

The complete classifications are 116 coordinator paths, 36 Windows client
paths, 35 macOS client paths, 110 verification/contract paths, 60 operational
or product documentation paths, 266 governance paths and one repository-context
path.

## B1-B7 and amendment coverage

| Gate | Repository seam reviewed | Final evidence boundary |
| --- | --- | --- |
| B1 | variant schema, hostile-input processing, range transport, cache bounds, generation barriers, drained end and disabled production registry | selected codec plus signed all-pairing physical timing/RSS matrix |
| B2 | one active Air, deduplicated peer union, queue ownership and join catch-up after readiness | real 3-Barycenter/5-Pulsar audible topology |
| B3 | saved-versus-active membership, switch confirmation, lifecycle authorization and state restoration | real living-Air session and operator observation |
| B4 | leave/dissolve fencing, leaver-only stop/fade, no old-overlay catch-up and personal-state restore | real leave-during-track audible evidence |
| B5 | opaque exact target selectors, no broadcast fallback, inbox eligibility/TTL/pagination and Telegram parity | adversarial packaged-app and real Telegram campaign |
| B6 | mixed-version denial, provider-neutral main flow, no partial target create, flag-off and rollback seams | selected codec plus real mixed-fleet campaign |
| B7 | canonical delete/disable/report protection, fetch/replay revocation and cache invalidation | real rights-abuse and revocation campaign |

The root amendments were applied as the controlling contract. Parked Airs stay
lazy; invite tokens are hashed/single-use and joins require confirmation;
legacy link and Air delivery cannot own one command concurrently. Inbox listing
or reconnect cannot autoplay, replay creates a newly resolved transmission,
and new members do not inherit old targets. Long tracks do not enter the Phase
1 speech chain. Range authorization and revocation are re-evaluated, and
accounting views consume canonical counters rather than a parallel metric
ledger.

## Semantic review by risk seam

### Codec, transport and processing

The no-go is preserved in schema, handoff and clients: no production stream
profile is selected, clients do not advertise `stream_track_v1`, and production
players reject an unadvertised command. Variant publication is atomic and
bounded by the track limit; processing keeps protocol, resource and output
bounds. The range path enforces authorization per request, bounded 1 MiB
responses, conditionals, integrity metadata, revocation and amplification
accounting. Candidate cache and player lifecycle tests cover seek generations,
fresh readiness, rebuffer, drain and purge.

The review found one local-toolchain portability defect in test fixture
generation: FFmpeg builds without external `libvorbis` could not create the OGG
live fixtures. The exact candidate now detects `libvorbis` and uses FFmpeg's
native experimental `vorbis` encoder only for tests when needed. Both focused
tests pass ten repetitions and the full coordinator race suite passes. No
production codec selection, encoder registry or runtime behavior changed.

### Air migration and concurrency

The predecessor backfill is deterministic and authority cutover is explicit.
Transactions are immediate and serialized; the Air admission limiter runs
before membership creation, and migration scan errors are distinguished from
missing rows. Runtime resolution owns one active Air and deduplicated peer
union, while leave/dissolve paths fence later work and preserve parked rows for
roll-forward. The previously reported limiter and scan-classification findings
were re-read against their focused regressions and remain fixed.

### Explicit targets, inbox and rights

Target selection returns opaque authorized references; internal numeric IDs
used for presence correlation are not exposed by the HTTP representation.
Resolution snapshots exact deduplicated nodes and rejects partial mixed-version
creation rather than broadcasting. Inbox pagination is stable and propagates
real database failures instead of treating them as cursor expiry. Actor-scoped
consent parsing rejects duplicate security-sensitive JSON fields. Report
protection is scoped to the reporter; canonical sender/moderator delete and
actor/orbit disable drive global fetch/replay denial and cache invalidation.

### Accounting, observability and operational state

Upload, processing, retained-storage and egress reservations derive from the
canonical store. Policy amplification is schema- and API-bounded to 1,000-4,000
milli; variants are capped at 500 MiB, so reservation multiplication remains
inside `int64`. Mutations use the serialized store transaction model and crash,
retry, refill and deletion paths reconcile reservations. The authenticated
operator view adds no user-controlled labels or tenant selectors. Public
health uses a lightweight snapshot and only enabled Phase 2 dependencies can
degrade readiness, preserving the flag-off Phase 1 boundary.

The frozen gate-matrix wording still calls observability
`engineering-pending`; it is a source-pinned historical contract consumed by
the independent reports. This review supersedes only that task status:
observability engineering is complete, while the sanitized real campaign and
accounting evidence remain manual-required. The frozen artifact is not
rewritten because doing so would invalidate its dependent review hashes.

## Independent findings and dispositions

| Review | Fixed and re-reviewed | Still open and production-blocking |
| --- | --- | --- |
| codec/supply | none | `P2-CODEC-SUPPLY-001` through `006` (six High) |
| stream performance | `P2-PERF-001` (High) | `P2-PERF-002`, `003`, `004` (three High) |
| Air migration | `P2-AIR-001` (High), `P2-AIR-002` (Medium) | `P2-AIR-003`, `004` (two High) |
| target/range/rights | `P2-TGT-001` (High), `P2-TGT-002` (Medium) | `P2-TGT-003`, `004` (two High) |

The six supply findings keep the codec choice at no-go. The performance design
still needs a bounded selected-codec whole-object integrity proof, a physical
matrix and independent sign-off. Air and target reviews still need their real
campaigns and independent sign-offs. These are 13 open High findings in total.
The inline root reviewer is not represented as any of the four required
implementation-independent approvers; Ivan Oparin remains the named approver
and approval status remains `required`.

## Verification rerun by root review

| Command or evidence | Result |
| --- | --- |
| `git diff --check c0e0d4a..5f2f7e9` | only 16 Markdown hard-break spaces in four mirrored technical-review artifacts; normalized with no semantic change |
| `cd coordinator && GOTOOLCHAIN=go1.25.12 go vet ./... && GOTOOLCHAIN=go1.25.12 go test -race -count=1 ./...` | pass on exact candidate, including command runtime in 157.500 s, media in 62.810 s and store in 283.986 s |
| same full vet/race command in `pulsar-win` | pass, all four packages |
| `cd node-app && DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test` | pass, 254 tests in 41 suites |
| acceptance contract discovery before the root packet | pass, 85/85 tests |
| focused OGG live fixtures | pass once under race and 10 repetitions with FFmpeg 8.1.2 lacking `libvorbis` |
| hosted main run `29506654140` | four jobs pass; this covers the pre-review main, while the committed root packet still requires its own hosted run |
| manifest regeneration plus root validator | deterministic, valid and zero unmapped paths |

The root packet itself must pass the clean full acceptance workflow and hosted
four-job CI after commit. Those immutable run identifiers belong in the task
outcome after publishing; they are not predicted in this review artifact.

## Residual risks and hard holds

1. No complete Windows+macOS codec/player combination passes all fixed supply,
   license, package and performance gates; B1/B6 and streamed-track production
   remain disabled.
2. Physical packaged-app, audible, real Telegram, migration/rollback, rights,
   Windows/macOS and beta evidence remains in `EPIC-260714-th54l3`.
3. Four independent approvals and other owner decisions remain in
   `EPIC-260714-zmnd4n`; the inline execution session cannot satisfy them.
4. The seven consecutive immutable 24-hour beta records have not started and
   no accepted production artifact exists.
5. The authenticated full operator observability view performs aggregate
   queries on the serialized store connection. This is acceptable for the
   disabled-production engineering baseline, but real beta capacity evidence
   must confirm its cadence does not starve mutations.

These holds reject Phase 2 promotion, but do not require reverting the exact
reviewed source tree or stopping the owner's authorized reversible engineering
sequence.
