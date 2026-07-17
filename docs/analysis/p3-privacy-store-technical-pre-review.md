# Phase 3 privacy and Store technical pre-review

- Date: 2026-07-17
- Original task: `TASK-260712-7ng1vs`
- Root-reviewed source: `d94f51644a3acf37601b4a869b4247380372f9ec`
- Root-reviewed tree: `4e4cca878db806650eda6f1e1642051b87a18b93`
- Engineering reviewer: `codex-inline-pre-reviewer`
- Accountable owner: Ivan Oparin
- Independent approval: `TASK-260717-35bll1`

## Decision

The repository technical pre-review is complete. No new Critical or High code,
privacy-copy or Store-copy finding was found in the frozen source. Reversible
strict-sequence engineering may move to `TASK-260712-6mz9xg`.

This is deliberately **not** an independent review, public-policy publication,
mail-delivery proof, real-app screenshot/WACK run or Partner Center action.
`e2ee_media`, `soundboard_cues`, `automation` and Phase 3 promotion remain
blocked. The fail-closed machine record is
`acceptance/phase3/privacy-store-technical-pre-review-v1.json`.

## Claim audit

| Surface | Repository conclusion |
| --- | --- |
| Encryption | Phase 1 EN/RU policy and listing copy explicitly say that coordinator-readable audio is not end-to-end encrypted. Deferred Phase 3 E2EE is not advertised and `e2ee_media` remains blocked. |
| Metadata | Policy names installation, membership, routing, receipt, report and restricted evidence metadata; it does not imply that coordinator-visible metadata is secret. |
| Delete and retention | Access revocation is immediate but physical cleanup is asynchronous. Recipient/integration copies, report or lawful holds, integrity records and pre-deletion metadata in expiring recovery points are explicit exceptions; no universal erase claim remains. |
| Recovery | Recovery requires surviving authorized authority. The text explicitly refuses to promise recovery after every authorized credential is lost. |
| Telegram | Telegram is optional, linked policy consent is enforced, and independently controlled Telegram copies are outside Pulsar deletion. |
| Report evidence | The report fixes one foreign evidence target while blocking only the reporter's local relationship. Evidence is capability-gated, report-bound, audited and expires after 30 days; list-only, mismatched and revoked operators fail closed. |
| UGC and Store | Source copy, IARC truth profile and certification templates are internally consistent. They remain an engineering package, not a portal record or submission-ready claim. |

## Approved defaults and remaining authority

`docs/compliance/legal-ops-inputs.json` is the canonical approved input. Ivan
Oparin is the accountable owner, EN/RU reviewer, Partner Center listing owner,
final submit authority and emergency withdrawal authority. Counsel review is
not required. The policy source hashes have decision `proceed`.

Those approvals do not prove external state. The current Store policy
pre-submit decision is correctly `hold`. All twelve screenshot entries remain
`manual-required` with empty hashes, and the Partner Center package remains
`engineering-ready-manual-hold`. No exact IARC portal export/rating, WACK
manifest, exact-build certification record or submission authorization is
claimed.

## Reproduced evidence

```text
(cd coordinator && go test -race -count=10 ./internal/store -run '<seven frozen moderation tests>')
(cd coordinator && go test -race -count=10 ./internal/moderation ./cmd/duet-coordinator -run '<five frozen moderation service/http tests>')
(cd coordinator && go test -race -count=10 ./internal/store ./cmd/duet-coordinator -run '<eight frozen content-policy tests>')
(cd coordinator && go test -tags previoushead -count=10 ./internal/store -run '^TestModerationExactPreviousHeadRollback$')
(cd pulsar-win && go test -race -count=10 ./... -run '<nine privacy/policy/E2EE-audit tests>')
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter 'PublicPolicyLinks|E2EEAuditContract|RecoveryExport|PhaseOneAppClient'
(cd coordinator && go test ./internal/legalops ./internal/policypack ./internal/policypublication ./internal/storelisting ./internal/storepolicy ./internal/moderation ./internal/moderationops)
```

The moderation store group passed race times ten in 119.540 seconds. The
service/HTTP and content-policy groups passed race times ten across two packages
each. Exact previous-head moderation rollback passed ten repetitions in 43.610
seconds. Windows passed four packages under race and ten repetitions. macOS
passed 25 tests in four suites on the available x86_64 macOS host. Static legal,
policy, listing and Store-policy validators passed while preserving the exact
external split: policy source `proceed`, Store submission `hold`.

## External closure

Ivan Oparin owns `TASK-260717-35bll1` for an implementation-independent review.
It consumes, rather than duplicates, mailbox provisioning in
`TASK-260714-200ib8`, Phase 1 Partner Center owner evidence in
`TASK-260715-24ube9`, and the later Phase 3 disclosure packet in
`TASK-260712-3b7bp4`. The reviewer must bind identity, independence, the exact
root-reviewed commit and artifact hashes, and must close and retest every
Critical or High finding.

Until live policy URLs, mailbox delivery, exact portal evidence and the
independent signature exist, affected Phase 3 flags and promotion remain held.
Any affected source, policy, listing, feature-flag, build or fixture delta
reopens root and privacy/Store review.
