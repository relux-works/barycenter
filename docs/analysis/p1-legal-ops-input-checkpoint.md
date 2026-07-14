# Phase 1 legal and operations input checkpoint

- Date: 2026-07-14
- Task: `TASK-260712-16zfvu`
- State: **approved by Ivan Oparin; legal/operations publication gate open**
- Machine-readable source: [`legal-ops-inputs.json`](../compliance/legal-ops-inputs.json)

This checkpoint separates facts observed in the repository or on an official
public page from facts approved for Pulsar policy, moderation operations and
Store submission. An observed candidate is not publishable until an authorized
person confirms the complete group, its accountable owner and the approval
record. No name, mailbox, jurisdiction, response promise or submission
authority is inferred.

## What is actually known

| Area | Recorded state | Remaining boundary |
| --- | --- | --- |
| Entity | Approved by Ivan Oparin: Relux Works LLC; registration `999.110.1559507`; tax ID `03036828`; registered address in Tsaghkadzor, Armenia; Relux Works LLC is the Pulsar data controller | None for this input group. |
| Contacts and URLs | Approved by Ivan Oparin: existing privacy/legal contacts, `support@barycenter.live`, `moderator@barycenter.live`, the existing Relux EN/RU policy routes and the four canonical future Pulsar legal routes | Future Pulsar routes still have to be published before use; approval of a canonical URL is not evidence that it is live. |
| General policies | Relux Works EN privacy policy and Terms of Use are approved source inputs; the Terms select Armenian law and courts | The existing text still does not describe Pulsar media upload, Telegram/Spotify integrations, retention, backups, target sharing, reports or moderation. Later policy tasks must create the product-specific text. |
| Product site | `https://barycenter.live/guide/` is live | On 2026-07-14, `/privacy`, `/terms` and `/support` returned homepage fallback bytes. The approved future `/legal/*` routes are not claimed published. |
| Partner Center | Approved by Ivan Oparin: product `9P26FDCWV1GC`, package identity `ReluxWorksLLC.PulsarBarycenter`, automation app `pulsar-store-ci` with Manager (Windows), and Ivan Oparin as account/listing/submit/withdrawal owner | None for this input group. |
| Hosting | Approved by Ivan Oparin: Relux Works LLC-operated Coolify infrastructure and S3-compatible object storage in the United States, with no hosting or backup subprocessors | None for this input group. A later provider change requires a new approval record and policy update before deployment. |
| Markets, age and disputes | Approved by Ivan Oparin: all lawful Microsoft Store markets, excluding sanctioned, embargoed or prohibited jurisdictions; age 13; Armenian law/courts; English controls | None for this input group. |
| Moderation operations | Approved by Ivan Oparin: he owns all primary/backup/escalation and mailbox roles; coverage is Monday-Friday 10:00-19:00 GMT+4; normal target is two business days and urgent removal target is 24 hours | None for this input group. |
| Policy review | Approved by Ivan Oparin: separate counsel review is not required; Ivan Oparin reviews both EN and RU; English controls and Russian must be semantically equivalent | None for this input group. |

The old [`docs/store-listing.md`](../store-listing.md) is a draft for the
Spotify-first product and says that Pulsar collects no personal data. It is not
an approved input for the self-contained media product and must not be
resubmitted unchanged.

## Approval record

Ivan Oparin approved the observed Relux Works candidates and named himself the
common accountable owner on 2026-07-14 at `2026-07-14T17:31:50+04:00`. The
response is normalized only where intent is unambiguous:

- missing `https://` schemes were added;
- the ordered privacy/Terms/support/content-guidelines label was normalized to
  `/legal/privacy`, `/legal/terms`, `/legal/support` and
  `/legal/content-guidelines` after the supplied list duplicated `/legal/terms`;
- `13+`, `eng`, `+4 gmt` and the spaced urgent mailbox were normalized to age
  13, `English`, `GMT+4` and `moderation-urgent@barycenter.live`;
- blank owner fields use the explicitly named common owner, Ivan Oparin.

No duration, market, provider, subprocessor or counsel decision was inferred
from an empty field or from a mailbox supplied in an SLA field. Ivan Oparin
explicitly approved the proposed best-effort defaults on 2026-07-14 at
`2026-07-14T17:52:59+04:00`:

- Relux Works LLC-operated Coolify and S3-compatible storage in the United
  States, with no hosting or backup subprocessors;
- all lawful Microsoft Store markets except sanctioned, embargoed or otherwise
  prohibited jurisdictions;
- Monday-Friday 10:00-19:00 GMT+4 moderation coverage, two business days for a
  normal report and 24 hours for urgent removal;
- no separate counsel review, with Ivan Oparin as the final EN and RU reviewer.

## Final approval checklist

All seven input groups are approved:

- [x] `legal_identity_and_controller` — approved by Ivan Oparin.
- [x] `contacts_and_public_urls` — approved by Ivan Oparin with the normalized
  canonical product URLs recorded above.
- [x] `hosting_and_data_locations` — approved by Ivan Oparin.
- [x] `markets_age_and_disputes` — approved by Ivan Oparin.
- [x] `moderation_ownership_and_response` — approved by Ivan Oparin.
- [x] `partner_center_and_submission` — approved by Ivan Oparin.
- [x] `policy_review_and_configuration` — approved by Ivan Oparin.

An approval must identify the approving person and timestamp. Silence, a
publicly visible candidate, repository access or ability to dispatch a GitHub
workflow is not approval.

## Fail-closed publication gate

The validator accepts the approved checkpoint in both structural and
approval-required modes:

```sh
cd coordinator
go run ./cmd/legal-ops-check ../docs/compliance/legal-ops-inputs.json
go run ./cmd/legal-ops-check --require-approved ../docs/compliance/legal-ops-inputs.json
```

The manual Store submission workflow runs the second command before it installs
submission tooling or downloads an MSIX. The gate now passes for this exact
approved record. It will fail closed again if any group, owner, approver,
timestamp or evidence is removed, or if a placeholder is introduced. Passing
this input gate does not claim that future policy pages are published or that a
Store package is otherwise ready to submit.

## Sources checked

- [Relux Works privacy policy](https://relux.works/en/privacy-policy/)
- [Relux Works Terms of Use](https://relux.works/en/terms-of-use/)
- [RU privacy route](https://relux.works/ru/privacy-policy/)
- [RU Terms route](https://relux.works/ru/terms-of-use/)
- [Pulsar guide](https://barycenter.live/guide/)
- [Microsoft Store product](https://apps.microsoft.com/detail/9P26FDCWV1GC)
- [`docs/decisions.md`](../decisions.md)
- [Store policy baseline](store-policy-baseline-2026-07-12.md)

Public URLs were rechecked on 2026-07-14. Public observation verifies only
what the page returned at that time; it does not grant legal or operational
approval.
