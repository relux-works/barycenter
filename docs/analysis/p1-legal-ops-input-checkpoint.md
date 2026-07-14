# Phase 1 legal and operations input checkpoint

- Date: 2026-07-14
- Task: `TASK-260712-16zfvu`
- State: **blocked on explicit external approval**
- Machine-readable source: [`legal-ops-inputs.json`](../compliance/legal-ops-inputs.json)

This checkpoint separates facts observed in the repository or on an official
public page from facts approved for Pulsar policy, moderation operations and
Store submission. An observed candidate is not publishable until an authorized
person confirms the complete group, its accountable owner and the approval
record. No name, mailbox, jurisdiction, response promise or submission
authority is inferred.

## What is actually known

| Area | Observed candidate | Why it is not sufficient approval |
| --- | --- | --- |
| Entity | Relux Works LLC; registration `999.110.1559507`; tax ID `03036828`; address in Tsaghkadzor, Armenia | These facts are published on the official Relux Works site, but reuse as Pulsar's data controller and policy owner still needs an explicit accountable approval. |
| General contacts | `privacy@relux.works` and `legal@relux.works` | No support or moderation mailbox owner, coverage or escalation roster is recorded. Mailbox existence and monitored ownership are not proven by a public string. |
| General policies | Relux Works EN privacy policy and Terms of Use; the Terms select Armenian law and courts | The privacy text does not describe Pulsar media upload, Telegram/Spotify integrations, retention, backups, target sharing, reports or moderation. The RU routes currently state that only English controls and render the English body rather than an equivalent Russian policy. |
| Product site | `https://barycenter.live/guide/` is live | On 2026-07-14, `/privacy`, `/terms` and `/support` returned HTTP 200 but their bytes had the same SHA-256 as `/`: `a65b869170a726b65ff9f8d31b8d00497511e284c92b8a6e3e0ba01014f1af80`. They are homepage fallbacks, not policy or support pages. |
| Partner Center | Product `9P26FDCWV1GC`, package identity `ReluxWorksLLC.PulsarBarycenter`, and automation app `pulsar-store-ci` with Manager (Windows) are recorded and the Store page is live | A service principal role is not human authority to approve listing claims, submit a release or withdraw one. Those owners are absent. |
| Hosting | Repository notes describe a Relux Works/Coolify deployment direction and a separate object-backup contract | The service owner, actual data country/region, backup provider/location and subprocessor disclosure are not approved in this repository. |

The old [`docs/store-listing.md`](../store-listing.md) is a draft for the
Spotify-first product and says that Pulsar collects no personal data. It is not
an approved input for the self-contained media product and must not be
resubmitted unchanged.

## One concise approval checklist

Reply with approval or corrected values for all seven groups. A person may own
more than one role; response times are objectives only if explicitly approved.

- [ ] `legal_identity_and_controller` — confirm or correct legal name,
  registration/tax numbers, registered address, Armenian jurisdiction and that
  Relux Works LLC is Pulsar's data controller; name the accountable owner.
- [ ] `contacts_and_public_urls` — approve privacy/legal contacts; give monitored
  support and moderation mailboxes plus canonical EN/RU privacy, Terms, Content
  Guidelines and support URLs; name each owner.
- [ ] `hosting_and_data_locations` — state the service/host operator, primary
  data country or region, backup provider and country or region, operational
  owner and any subprocessors that policy must disclose.
- [ ] `markets_age_and_disputes` — approve target/excluded markets, minimum age,
  Armenian governing law/courts and which language controls.
- [ ] `moderation_ownership_and_response` — name primary, backup and escalation
  operators; mailbox owners; coverage timezone/hours; normal-report and urgent
  Microsoft-removal response objectives.
- [ ] `partner_center_and_submission` — name the Partner Center account owner,
  listing-asset owner, final submit authority and emergency withdrawal
  authority; confirm the observed product/identity/automation role.
- [ ] `policy_review_and_configuration` — say whether counsel reviews EN and RU,
  name reviewers, approve the controlling-language rule, and confirm which
  facts are checked in versus runtime-configurable.

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
