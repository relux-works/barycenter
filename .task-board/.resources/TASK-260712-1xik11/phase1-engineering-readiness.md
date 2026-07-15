# Phase 1 engineering readiness and manual handoff

- Task: `TASK-260712-1xik11`
- Decision: ready for reversible P2 engineering only
- Engineering source candidate: `16420c2ce652d05d534fb45b5ef9a7124d4bbdd6`
- Root-review packet: `4c79d12bb2982f6916d8e612b7d6d50a3732ee2f`
- Machine authority: [`acceptance/phase1-engineering-readiness.json`](../../acceptance/phase1-engineering-readiness.json)

## What is accepted

The Phase 1 source candidate and its root-review packet are accepted as the
reversible engineering baseline for P2 coding. The complete root manifest maps
737 no-renames path entries to task acceptance criteria and A1-A8, and the
technical reviews have no unresolved critical or high engineering finding.
Local clean acceptance passed all 12 commands; hosted coordinator, Swift,
Windows and signed-probe jobs passed.

This handoff does not accept Phase 1 as a product, authorize Store submission,
or claim that a production package exists. The hosted MSIX is a test-signed
AppContainer probe with an ephemeral certificate. It proves package build,
temporary trust, install and cleanup on a hosted Windows runner; it is not the
production Pulsar package, WACK, real Windows 10/11 evidence or a Store result.

## Exact provenance

The PR run is `29408109562`. GitHub's Actions API binds it to head
`4c79d12bb2982f6916d8e612b7d6d50a3732ee2f` and base
`16420c2ce652d05d534fb45b5ef9a7124d4bbdd6`. The downloaded acceptance
manifests correctly record checked-out synthetic merge-ref
`cfcfc0cee6fbb07cf7ff2a92a3c1ca93ed29665c`; it is not mislabeled as an
exact-head checkout.

The run exposes four immutable artifact IDs:

| Artifact | ID | Boundary |
| --- | ---: | --- |
| `phase1-acceptance-coordinator` | `8339941164` | vet, full tests, moderation contract and exact predecessor rollback |
| `phase1-acceptance-swift` | `8339923177` | pinned Xcode/Swift complete package tests |
| `phase1-acceptance-windows` | `8339938363` | vet, full/race tests and amd64/arm64 cross-build |
| `pulsar-signed-msix-probe` | `8339976642` | test signing, hosted install and exact cleanup only |

The probe package SHA-256 is
`df80eed7fa00852bd6079da8ba134a6ee341d3215c6d49356e50c1788258c51d`.
Its identity is `ReluxWorksLLC.PulsarBarycenter`, version `0.1.0.0`, and signer
is the ephemeral subject `CN=60105954-A0D9-4E89-B32D-18AF2F423ABE`.
No private signing material is in the artifact.

The readiness rerun also passed the approved legal-input, exact policy-pack,
live public policy hash/cache, default Store policy/listing and moderation
contracts. Exact Go 1.25.12 vulnerability scans reported no vulnerabilities in
either Go module. Two negative gates failed exactly as required:
`store-listing-check --require-ready` stopped on absent real screenshots, and
`moderation-ops-check --require-mail-ready` stopped on external mailbox
routing. Those failures are recorded as holds, not waived checks.

Re-download while GitHub retention still permits it:

```sh
gh run download 29408109562 --dir .temp/p1-readiness-artifacts
python3 scripts/acceptance/validate_phase1_readiness.py
```

The committed index stores the artifact IDs and hashes of all three acceptance
manifests plus the package, install and cleanup receipts. Raw logs and the MSIX
stay in the access-controlled workflow artifact, not Git.

## A1-A8 handoff

Every scenario retains deterministic engineering coverage and an explicit
hands-on result of `manual-required`:

| Scenario | Manual owner task(s) |
| --- | --- |
| A1 accountless clean install and local flow | `TASK-260712-1vtwkl`, `TASK-260712-e5mfqj` |
| A2 two physical Pulsars and ≤100 ms skew | `TASK-260712-1vtwkl`, `TASK-260712-2hodti` |
| A3 real overlay, routes and ≤200 ms continuity | `TASK-260712-2hodti` |
| A4 real interrupt and ≤500 ms audible resume | `TASK-260712-2hodti` |
| A5 real Telegram FIFO/callback/too-late behavior | `TASK-260712-e5mfqj` |
| A6 real DND/offline/late-reconnect observation | `TASK-260712-2hodti`, `TASK-260712-e5mfqj` |
| A7 installed-app upload/reconnect/draft recovery | `TASK-260712-e5mfqj` |
| A8 screenshots, WACK and Store reviewer path | `TASK-260712-1vtwkl`, `TASK-260712-e5mfqj` |

The strict P1 manual order in `EPIC-260714-th54l3` is:

1. `TASK-260712-1vtwkl` — signed Windows 10/11 H00-H17 matrix;
2. `TASK-260712-2hodti` — real Windows/macOS A2/A3/A4/A6 audio evidence;
3. `TASK-260712-e5mfqj` — UI/accessibility/DPI, A1/A5/A7, twelve EN/RU
   screenshots and WACK on the exact production candidate.

No task currently has a manual pass. A failed observation reopens or creates
engineering work; it is never normalized into an acceptance exception.

## External holds

The owner ledger `EPIC-260714-zmnd4n`, common owner Ivan Oparin, retains:

- inbound routing and synthetic delivery for the three public mailboxes;
- genuinely independent protocol, realtime-audio, migration and security
  signatures;
- exact signed production MSIX identity, raw certification findings, IARC
  export/rating, screenshot/WACK consumption, live policy delta and owner
  `proceed`.

Until those tasks and the P1 manual story pass, `releaseAccepted`,
`storeSubmissionAuthorized` and `partnerCenterMutated` remain `false`. P2 may
consume the engineering APIs and tests, but no public release claim may consume
this handoff as a substitute for product acceptance.
