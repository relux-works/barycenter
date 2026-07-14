# Phase 1 legal and operations input checkpoint

- Date: 2026-07-14
- Task: `TASK-260712-16zfvu`
- State: **partially approved; publication blocked on four unresolved groups**
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
| Hosting | Ivan Oparin approved the United States as the primary and backup data region and is the common service/operational owner | `USA` identifies a location, not the host or backup provider. Actual provider/operator entities and any subprocessors remain unresolved. |

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

No duration, market, provider, subprocessor or counsel decision is inferred
from an empty field or from a mailbox supplied in an SLA field.

## One concise remaining approval checklist

Three input groups are approved. Four remain open:

- [x] `legal_identity_and_controller` — approved by Ivan Oparin.
- [x] `contacts_and_public_urls` — approved by Ivan Oparin with the normalized
  canonical product URLs recorded above.
- [ ] `hosting_and_data_locations` — state the service/host operator, primary
  provider/operator entity, backup provider entity and subprocessors, or say
  explicitly that there are no subprocessors. United States regions and Ivan
  Oparin's ownership are already recorded.
- [ ] `markets_age_and_disputes` — state target and excluded markets. Age 13,
  Armenian law/courts and English control are already recorded.
- [ ] `moderation_ownership_and_response` — state coverage hours and numeric
  normal-report and urgent Microsoft-removal response objectives. Ivan Oparin,
  `GMT+4` and the moderation mailboxes are already recorded.
- [x] `partner_center_and_submission` — approved by Ivan Oparin.
- [ ] `policy_review_and_configuration` — choose whether counsel review is
  required. Ivan Oparin is already recorded as both EN and RU reviewer.

An approval must identify the approving person and timestamp. Silence, a
publicly visible candidate, repository access or ability to dispatch a GitHub
workflow is not approval.

## Fail-closed publication gate

The validator accepts this file as a well-formed blocked checkpoint, while the
approval mode fails and lists unresolved group IDs:

```sh
cd coordinator
go run ./cmd/legal-ops-check ../docs/compliance/legal-ops-inputs.json
go run ./cmd/legal-ops-check --require-approved ../docs/compliance/legal-ops-inputs.json
```

The manual Store submission workflow runs the second command before it installs
submission tooling or downloads an MSIX. Therefore a missing approval blocks
the external side effect without blocking ordinary engineering CI. Approved
values must contain no placeholder token and must have an owner, approver,
timestamp and evidence.

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
